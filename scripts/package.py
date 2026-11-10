#!/usr/bin/env python3
"""Package the current project files, including uncommitted changes."""
import argparse
from pathlib import Path
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', default='artifacts/support-desk.tar.gz')
args = parser.parse_args()
output = (ROOT / args.output).resolve()
output.parent.mkdir(parents=True, exist_ok=True)
files = subprocess.check_output(['git', 'ls-files', '-c', '-o', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
with tarfile.open(output, 'w:gz') as archive:
    for name in sorted(set(files)):
        if not name or name.startswith(('artifacts/', '.agents/', '.idea/', '.aws/', '.codex/')):
            continue
        path = ROOT / name
        if path.name.startswith('.env') and path.name != '.env.example':
            continue
        if not path.is_file() or path.resolve() == output:
            continue
        archive.add(path, arcname=name)
print(output)
