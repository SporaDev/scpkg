const https = require('https');
const fs = require('fs');
const path = require('path');

const pkg = require('../package.json');
const VERSION = `v${pkg.version}`;
const REPO = 'SporaDev/scpkg';

const platformMap = {
  win32: 'windows',
  linux: 'linux',
  darwin: 'darwin'
};

const archMap = {
  x64: 'amd64',
  arm64: 'arm64'
};

const osName = platformMap[process.platform];
const archName = archMap[process.arch];

if (!osName || !archName) {
  process.exit(0);
}

const ext = osName === 'windows' ? '.exe' : '';
const binName = `scpkg-${osName}-${archName}${ext}`;
const url = `https://github.com/${REPO}/releases/download/${VERSION}/${binName}`;

const targetDir = path.join(__dirname, '..', 'bin');
const targetFile = path.join(targetDir, `scpkg${ext}`);

if (!fs.existsSync(targetDir)) {
  fs.mkdirSync(targetDir, { recursive: true });
}

function download(downloadUrl) {
  https.get(downloadUrl, (res) => {
    if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
      download(res.headers.location);
      return;
    }
    if (res.statusCode !== 200) {
      return;
    }
    const file = fs.createWriteStream(targetFile);
    res.pipe(file);
    file.on('finish', () => {
      file.close();
      if (osName !== 'windows') {
        try { fs.chmodSync(targetFile, 0o755); } catch (_) {}
      }
    });
  }).on('error', () => {});
}

download(url);
