import os
import subprocess
import tempfile
import unittest
from pathlib import Path


class ReleasePublishTest(unittest.TestCase):
    def test_publish(self):
        workflow = (Path(__file__).resolve().parents[1] / '.github/workflows/desktop.yml').read_text()
        script = workflow[workflow.index('          EXISTING_RELEASE='):]
        script = '\n'.join(line[10:] for line in script.splitlines())
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            gh = root / 'gh'
            gh.write_text('''#!/bin/bash
printf '%s\\n' "$*" >> "$CALL_LOG"
if [ "$1" = api ]; then
  if [ "$RELEASE_STATE" = error ]; then exit 1; fi
  if [ "$RELEASE_STATE" = existing ]; then echo 123; fi
fi
''')
            gh.chmod(0o755)
            for state in ('existing', 'missing', 'error'):
                with self.subTest(state=state):
                    log = root / 'calls'
                    log.write_text('')
                    env = dict(os.environ, PATH=f'{root}:{os.environ["PATH"]}',
                               CALL_LOG=str(log), RELEASE_STATE=state,
                               GITHUB_REPOSITORY='owner/repo', GITHUB_REF_NAME='v2.1.0')
                    result = subprocess.run(['bash', '-c', 'set -euo pipefail\n'
                                             'ASSETS=("installer with spaces.dmg")\n'
                                             'RELEASE_FLAGS=(--verify-tag)\n' + script],
                                            env=env, capture_output=True, text=True)
                    calls = log.read_text()
                    if state == 'error':
                        self.assertNotEqual(result.returncode, 0)
                        self.assertNotIn('release ', calls)
                    else:
                        self.assertEqual(result.returncode, 0, result.stderr)
                        action = 'upload' if state == 'existing' else 'create'
                        self.assertIn(f'release {action} v2.1.0 installer with spaces.dmg', calls)
                        self.assertEqual(calls.count('release '), 1)
                        if state == 'existing':
                            self.assertIn('--clobber', calls)


if __name__ == '__main__':
    unittest.main()
