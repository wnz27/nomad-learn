import ApplicationSerializer from './application';

// TODO: MAYBE JUST RETURN AN ID??
export default class AuthMethod extends ApplicationSerializer {
  primaryKey = 'Name';
}
