import { inject as service } from '@ember/service';
import { reads } from '@ember/object/computed';
import Controller from '@ember/controller';
import { getOwner } from '@ember/application';
import { alias } from '@ember/object/computed';
import { action } from '@ember/object';
import classic from 'ember-classic-decorator';
import { task } from 'ember-concurrency';

@classic
export default class Tokens extends Controller {
  @service token;
  @service store;

  @reads('token.secret') secret;

  tokenIsValid = false;
  tokenIsInvalid = false;
  @alias('token.selfToken') tokenRecord;

  resetStore() {
    this.store.unloadAll();
  }

  @task(function* (authMethod) {
    console.log('authMethod');
    console.log(authMethod.name);

    // TODO: ALLOW THIS ON THE BACKEND
    let redirectUri = `${window.location.origin}/ui/oidc/callback`;
    let nonce = this._generateNonce();

    console.log(redirectUri);
    console.log(nonce);

    this._storeOIDCAuthData(nonce, authMethod.name);

    // Make a request to get the auth method URL

    let authParams = {
      AuthMethod: authMethod.name,
      RedirectUri: redirectUri,
      ClientNonce: nonce,
    };

    let authURL = yield this.store.adapterFor('token').getAuthURL(authParams);

    yield window.location.replace(authURL);
  })
  authButtonClick;

  @action
  clearTokenProperties() {
    this.token.setProperties({
      secret: undefined,
    });
    this.setProperties({
      tokenIsValid: false,
      tokenIsInvalid: false,
    });
    // Clear out all data to ensure only data the anonymous token is privileged to see is shown
    this.resetStore();
    this.token.reset();
  }

  @action
  verifyToken() {
    const { secret } = this;
    const TokenAdapter = getOwner(this).lookup('adapter:token');

    this.set('token.secret', secret);

    TokenAdapter.findSelf().then(
      () => {
        // Clear out all data to ensure only data the new token is privileged to see is shown
        this.resetStore();

        // Refetch the token and associated policies
        this.get('token.fetchSelfTokenAndPolicies').perform().catch();

        this.setProperties({
          tokenIsValid: true,
          tokenIsInvalid: false,
        });
      },
      () => {
        this.set('token.secret', undefined);
        this.setProperties({
          tokenIsValid: false,
          tokenIsInvalid: true,
        });
      }
    );
  }

  // Generate a 20-char nonce, using window.crypto to
  // create a sufficiently-large output then trimming
  _generateNonce() {
    let randomArray = new Uint32Array(10);
    window.crypto.getRandomValues(randomArray);
    return randomArray.join('').slice(0, 20);
  }

  // Store OIDC Data in LocalStorage, this gets cleaned up on authentication
  _storeOIDCAuthData(nonce, authMethod) {
    window.localStorage.setItem('nomadOIDCNonce', nonce);
    window.localStorage.setItem('nomadOIDCAuthMethod', authMethod);
  }
}
