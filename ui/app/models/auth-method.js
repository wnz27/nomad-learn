import Model from '@ember-data/model';
import { attr } from '@ember-data/model';
import { fragment } from 'ember-data-model-fragments/attributes';

export default class AuthMethod extends Model {
  @attr('string') name;
  @attr('string') type;
  @fragment('structured-attributes') config;
  @attr() rulesJSON;
}
