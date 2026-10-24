import Controller from '@ember/controller';
import { tracked } from '@glimmer/tracking';

export default class ApplicationController extends Controller {
  queryParams = ['doc', 'meeting', 'person', 'vorlage'];

  @tracked doc = '';
  @tracked meeting = '';
  @tracked person = '';
  @tracked vorlage = '';
}
