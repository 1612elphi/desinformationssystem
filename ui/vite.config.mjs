import { defineConfig } from 'vite';
import { extensions, classicEmberSupport, ember } from '@embroider/vite';
import { babel } from '@rollup/plugin-babel';

export default defineConfig({
  plugins: [
    classicEmberSupport(),
    ember(),
    babel({
      babelHelpers: 'runtime',
      extensions,
    }),
  ],
  server: {
    proxy: {
      '/api': 'http://localhost:3650',
      '/health': 'http://localhost:3650',
      '/map/karlsruhe.pmtiles': 'http://localhost:3650',
    },
  },
});
