#!/usr/bin/env python3
"""Run all Go tests and include coverage from the real CLI integration process."""
import os
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parent.parent
output = root / '.cache' / 'coverage'
output.mkdir(parents=True, exist_ok=True)
blocks = {}

def merge(filename):
    for line in filename.read_text().splitlines()[1:]:
        span, statements, count = line.split()
        previous = blocks.get(span, (int(statements), 0))
        if previous[0] != int(statements):
            raise RuntimeError('inconsistent source versions for ' + span)
        blocks[span] = (int(statements), max(previous[1], int(count) > 0))

with tempfile.TemporaryDirectory(prefix='run-', dir=output) as temporary:
    run = Path(temporary)
    binary = run / 'binary'
    binary.mkdir()
    env = dict(os.environ, ROCKETWIND_COVERDIR=str(binary))
    subprocess.run(['go', 'test', '-count=1', '-coverpkg=./...',
                    '-coverprofile=' + str(run / 'packages.out'), './...'],
                   cwd=root, env=env, check=True)
    subprocess.run(['go', 'tool', 'covdata', 'textfmt', '-i=' + str(binary),
                    '-o=' + str(run / 'binary.out')], cwd=root, check=True)
    merge(run / 'packages.out')
    merge(run / 'binary.out')

profile = output / 'combined.out'
profile.write_text('mode: set\n' + ''.join(
    f'{span} {statements} {int(count > 0)}\n'
    for span, (statements, count) in sorted(blocks.items())))
total = sum(statements for statements, count in blocks.values())
covered = sum(statements for statements, count in blocks.values() if count)
print(f'{covered}/{total} statements covered ({100 * covered / total:.3f}%)')
print(f'Profile: {profile}')
for span, (statements, count) in sorted(blocks.items()):
    if statements and not count:
        print(f'Uncovered: {span} ({statements} statements)')
raise SystemExit(0 if covered == total else 1)
