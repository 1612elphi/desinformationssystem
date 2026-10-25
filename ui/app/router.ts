import EmberRouter from '@embroider/router';
import config from 'ui/config/environment';

export default class Router extends EmberRouter {
  location = config.locationType;
  rootURL = config.rootURL;
}

Router.map(function () {
  this.route('suche');
  this.route('sitzungen');
  this.route('gremien');
  this.route('statistik');
  this.route('reden');
  this.route('karte');
});
