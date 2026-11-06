#!/usr/bin/env python3
"""
Purpose: Check launcher cleanup when restoring the owned fixture rate fails.
Used by: The focused Phase 1A handoff verification.
Responsibilities: Exercise the real profile finally path with synthetic process, log and samples.
Important: Never launches an app, contacts a cluster or writes fixture state.
"""
import importlib.util
import io
import json
import sys
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('measure_packaged', Path(__file__).with_name('measure-packaged.py'))
measure = importlib.util.module_from_spec(spec)
with patch.object(sys, 'dont_write_bytecode', True):
    spec.loader.exec_module(measure)


class OwnedProcess:
    pid = 12345

    def __init__(self, fail_stop):
        self.stopped = False
        self.fail_stop = fail_stop

    def poll(self):
        return None

    def terminate(self):
        self.stopped = True

    def wait(self, timeout):
        if self.fail_stop:
            raise RuntimeError('injected process stop failure')


for fail_stop in (False, True):
    process = OwnedProcess(fail_stop)
    log = io.BytesIO()
    rates = []

    def launch(profile):
        measure.SAMPLES.write_text('\n'.join(json.dumps({'ui': {'phase': 1, 'elapsedMs': elapsed}}) for elapsed in (0, 120000)))
        return process, log, 1

    def rate(value):
        rates.append(value)
        if value == 100:
            raise ValueError('injected rate restoration failure')

    with TemporaryDirectory() as directory:
        state = Path(directory)
        with patch.multiple(measure, STATE=state, SAMPLES=state / 'samples.jsonl', launch=launch, rate=rate), patch.object(measure.time, 'monotonic', return_value=0), patch.object(measure.time, 'sleep', side_effect=RuntimeError('injected profile interruption')), patch.object(measure.subprocess, 'check_output', return_value=b'1'):
            try:
                measure.profile()
            except (RuntimeError, ValueError) as error:
                errors = []
                while error is not None:
                    errors.append(str(error))
                    error = error.__context__
            else:
                raise AssertionError('Expected the injected profile failure')
    assert process.stopped, 'Fixture restoration failure skipped owned process stop'
    assert log.closed, 'Fixture restoration failure skipped owned log close'
    assert rates == [2000, 100], rates
    assert 'injected rate restoration failure' in errors, errors
    assert 'injected profile interruption' in errors, errors
    if fail_stop:
        assert 'injected process stop failure' in errors, errors
print('PASS: restoration failure always attempts owned process/log cleanup and preserves errors')
