import { build } from 'esbuild';
import { copyFileSync, mkdirSync } from 'node:fs';

mkdirSync('dist', { recursive: true });

await build({
  entryPoints: ['src/app.js'],
  bundle: true,
  minify: true,
  sourcemap: true,
  format: 'iife',
  target: ['es2020'],
  outfile: 'dist/bundle.js',
});

// static assets served as-is alongside the bundle
copyFileSync('src/index.html', 'dist/index.html');

console.log('frontend bundled -> dist/');
