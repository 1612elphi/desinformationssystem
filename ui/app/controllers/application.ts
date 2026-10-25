import Controller from '@ember/controller';
import { tracked } from '@glimmer/tracking';

export default class ApplicationController extends Controller {
  // doc, meeting, person and vorlage are old single-overlay links; the route turns them into a stack
  queryParams = ['stack', 'doc', 'meeting', 'person', 'vorlage'];

  @tracked stack = '';
  @tracked doc = '';
  @tracked meeting = '';
  @tracked person = '';
  @tracked vorlage = '';
}
