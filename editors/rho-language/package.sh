#!/bin/sh
set -eu

cd "$(dirname "$0")"
python3 - <<'PY'
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile

root = Path('.')
output = root / 'rho-language-0.1.0.vsix'
content_types = '''<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="json" ContentType="application/json" />
  <Default Extension="xml" ContentType="text/xml" />
  <Default Extension="md" ContentType="text/markdown" />
  <Override PartName="/extension.vsixmanifest" ContentType="text/xml" />
</Types>
'''
manifest = '''<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011" xmlns:d="http://schemas.microsoft.com/developer/vsx-schema-design/2011">
  <Metadata>
    <Identity Id="robohubplatform.rho-language" Version="0.1.0" Language="en" Publisher="robohubplatform" />
    <DisplayName>Rho Language Support</DisplayName>
    <Description xml:space="preserve">Language registration and syntax highlighting for Rho.</Description>
    <Categories>Programming Languages</Categories>
    <Tags>rho</Tags>
    <Properties>
      <Property Id="Microsoft.VisualStudio.Code.Engine" Value=">=1.80.0" />
    </Properties>
  </Metadata>
  <Installation><InstallationTarget Id="Microsoft.VisualStudio.Code" /></Installation>
  <Dependencies />
  <Assets>
    <Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" />
  </Assets>
</PackageManifest>
'''
with ZipFile(output, 'w', ZIP_DEFLATED) as archive:
    archive.writestr('[Content_Types].xml', content_types)
    archive.writestr('extension.vsixmanifest', manifest)
    for path in [Path('package.json'), Path('language-configuration.json'), Path('README.md'), Path('syntaxes/rho.tmLanguage.json')]:
        archive.write(path, Path('extension') / path)
print(output.resolve())
PY
