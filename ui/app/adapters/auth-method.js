import { default as ApplicationAdapter } from './application';

export default class AuthMethodAdapter extends ApplicationAdapter {
  pathForType = () => 'auth_methods';
}
