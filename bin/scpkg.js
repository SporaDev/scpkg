#!/usr/bin/env node
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');

const ext = process.platform === 'win32' ? '.exe' : '';
const binPath = path.join(__dirname, `scpkg${ext}`);

if (!fs.existsSync(binPath)) {
  console.error('scpkg: native binary not found. please run "npm install" or compile from source with Go.');
  process.exit(1);
}

const result = spawnSync(binPath, process.argv.slice(2), { stdio: 'inherit' });
process.exit(result.status ?? 0);
