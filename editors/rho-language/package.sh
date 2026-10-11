#!/bin/sh
set -eu

cd "$(dirname "$0")"

# Install dependencies and compile if needed
if [ ! -d "node_modules" ] || [ ! -f "out/extension.js" ]; then
    npm install
    npm run compile
fi

python3 - <<'PY'
import json
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile

root = Path('.')
manifest_json = json.loads((root / 'package.json').read_text())
name = manifest_json['name']
version = manifest_json['version']
publisher = manifest_json['publisher']
engine = manifest_json['engines']['vscode'].lstrip('^')
output = root / ('%s-%s.vsix' % (name, version))
content_types = '''<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="json" ContentType="application/json" />
  <Default Extension="xml" ContentType="text/xml" />
  <Default Extension="md" ContentType="text/markdown" />
  <Default Extension="js" ContentType="application/javascript" />
  <Override PartName="/extension.vsixmanifest" ContentType="text/xml" />
</Types>
'''
manifest = '''<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011" xmlns:d="http://schemas.microsoft.com/developer/vsx-schema-design/2011">
  <Metadata>
    <Identity Id="{publisher}.{name}" Version="{version}" Language="en" Publisher="{publisher}" />
    <DisplayName>{display}</DisplayName>
    <Description xml:space="preserve">{description}</Description>
    <Categories>Programming Languages</Categories>
    <Tags>rho</Tags>
    <Properties>
      <Property Id="Microsoft.VisualStudio.Code.Engine" Value=">={engine}" />
    </Properties>
  </Metadata>
  <Installation><InstallationTarget Id="Microsoft.VisualStudio.Code" /></Installation>
  <Dependencies />
  <Assets>
    <Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" />
  </Assets>
</PackageManifest>
'''
manifest = manifest.format(
    publisher=publisher, name=name, version=version, engine=engine,
    description=manifest_json.get('description', ''),
    display=manifest_json.get('displayName', name),
)

with ZipFile(output, 'w', ZIP_DEFLATED) as archive:
    archive.writestr('[Content_Types].xml', content_types)
    archive.writestr('extension.vsixmanifest', manifest)
    
    # Copy all extension files. `out` is copied whole: the client is several
    # modules, and shipping only the entry point made an extension that could
    # not require the ones beside it.
    for path in [
        Path('package.json'),
        Path('language-configuration.json'),
        Path('README.md'),
        Path('syntaxes/rho.tmLanguage.json'),
        Path('out'),
    ]:
        if path.is_dir():
            for f in path.rglob('*'):
                if f.is_file():
                    archive.write(f, Path('extension') / f.relative_to(root))
        else:
            archive.write(path, Path('extension') / path)

    # The language client and everything it requires travel with the extension.
    # The whole tree is copied rather than a hand-written list, because that
    # list was wrong: a transitive dependency it named by hand was missing, and
    # the extension failed to activate with a module it could not find. A
    # package a little larger is worth an extension that starts.
    node_modules = root / 'node_modules'
    if node_modules.exists():
        for f in node_modules.rglob('*'):
            if f.is_file() and '__pycache__' not in f.parts:
                archive.write(f, Path('extension/node_modules') / f.relative_to(node_modules))

print(output.resolve())
PY