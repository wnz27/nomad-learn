import { inject as service } from '@ember/service';
import { getOwner } from '@ember/application';
import { get, set } from '@ember/object';
import Route from '@ember/routing/route';
// import RSVP from 'rsvp';

// TODO: GIVE THIS FILE A NICER LOCATION
export default class CallbackRoute extends Route {
  @service token;
  @service store;

  queryParams = {
    code: {
      refreshModel: true,
    },
    state: {
      refreshModel: true,
    },
  };

  async model(params) {
    let token = await this.store.adapterFor('token').completeOidc(params);
    await this.verifyToken(token);
    return window.location.replace('/ui');
  }

  verifyToken(token) {
    const TokenAdapter = getOwner(this).lookup('adapter:token');
    this.token.secret = token;

    return TokenAdapter.findSelf().then(
      () => {
        // Clear out all data to ensure only data the new token is privileged to see is shown
        this.store.unloadAll();
        this.token.fetchSelfTokenAndPolicies.perform().catch();
      },
      () => {
        this.token.secret = undefined;
      }
    );
  }
}
