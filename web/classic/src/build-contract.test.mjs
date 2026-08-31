import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

test('Classic entry imports the Semi UI stylesheet through its exported path', () => {
  const source = readFileSync(new URL('./index.jsx', import.meta.url), 'utf8');
  assert.match(source, /@douyinfe\/semi-ui\/lib\/es\/_base\/base\.css/);
  assert.doesNotMatch(source, /@douyinfe\/semi-ui\/dist\/css\/semi\.css/);
});

test('Classic icon registry imports brand icons exported by react-icons', () => {
  const source = readFileSync(
    new URL('./helpers/render.jsx', import.meta.url),
    'utf8',
  );
  assert.match(source, /FaLinkedin/);
  assert.match(source, /FaSlack/);
  assert.doesNotMatch(source, /SiLinkedin/);
  assert.doesNotMatch(source, /SiSlack/);
});
