import { inject as service } from '@ember/service';
import Route from '@ember/routing/route';
import RSVP from 'rsvp';

export default class TokensRoute extends Route {
  @service store;

  queryParams = {
    qpNamespace: {
      refreshModel: true,
    },
  };

  model() {
    return RSVP.hash({
      authMethods: this.store.query('auth-method', {}),
    });
  }
}
