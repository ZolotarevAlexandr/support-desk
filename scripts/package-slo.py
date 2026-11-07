#!/usr/bin/env python3
"""Package current lab source and a real recording; omit private local files."""
import argparse
from pathlib import Path
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--video', default='screencast.mp4')
parser.add_argument('--output', default='artifacts/support-desk.tar.gz')
args = parser.parse_args()
video = ROOT / args.video
if not video.is_file() or video.stat().st_size < 1024:
    raise SystemExit(f'Provide a real screencast at {video} before packaging')
subprocess.run(['ffprobe', '-v', 'error', '-select_streams', 'v:0', '-show_entries', 'stream=codec_type', str(video)], check=True, capture_output=True)
output = ROOT / args.output
output.parent.mkdir(parents=True, exist_ok=True)
files = subprocess.check_output(['git', 'ls-files', '-c', '-o', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
with tarfile.open(output, 'w:gz') as archive:
    for name in sorted(set(files)):
        if not name or name.startswith(('artifacts/', '.agents/', '.idea/')) or name == 'scripts/package-slo.py':
            continue
        path = ROOT / name
        if not path.is_file() or name == 'screencast.mp4':
            continue
        archive.add(path, arcname=name)
    archive.add(video, arcname='screencast.mp4')
print(output)
