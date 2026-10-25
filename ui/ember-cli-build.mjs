import EmberApp from 'ember-cli/lib/broccoli/ember-app.js';
import { compatBuild } from '@embroider/compat';

export default async function (defaults) {
  const { buildOnce } = await import('@embroider/vite');

  const app = new EmberApp(defaults, {
    // Add options here
  });

  // app/map is imported dynamically by the Karte view; left out of the entry
  // so MapLibre stays in its own chunk
  return compatBuild(app, buildOnce, { staticAppPaths: ['map'] });
}
