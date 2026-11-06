#!/usr/bin/env python3
"""
Purpose: Measure actual packaged renderer starts and the approved local load profile.
Used by: Phase 1A packaged acceptance evidence collection.
Responsibilities: Launch the app with a sanitized environment, read frontend-written numeric evidence and sample native RSS.
Important: The only cluster write is a fixed rate change on the journal-owned disposable B fixture; never stop a cluster or use ambient kubeconfig.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parent.parent
STATE = ROOT / '.local' / 'phase1a'
APP = ROOT / 'bin' / 'kubesift.app' / 'Contents' / 'MacOS' / 'kubesift'
READY = STATE / 'frontend-ready.json'
SAMPLES = STATE / 'renderer-measurements.jsonl'
ENV = {key: os.environ[key] for key in ('HOME', 'TMPDIR', 'USER', 'LOGNAME', 'LANG', '__CF_USER_TEXT_ENCODING') if key in os.environ}
ENV.update(PATH='/usr/bin:/bin:/usr/sbin:/sbin', KUBESIFT_TEST_KUBECONFIG=str(STATE / 'kubeconfig'))


def launch(profile=False):
    READY.unlink(missing_ok=True)
    env = dict(ENV)
    if profile:
        env['KUBESIFT_VALIDATION_PROFILE'] = '1'
    log = open(STATE / ('profile-native.log' if profile else 'startup-native.log'), 'ab')
    start = time.perf_counter()
    process = subprocess.Popen([str(APP)], cwd='/', env=env, stdout=log, stderr=subprocess.STDOUT)
    until = time.monotonic() + 30
    while not READY.exists():
        if process.poll() is not None or time.monotonic() > until:
            process.terminate()
            raise RuntimeError('Packaged renderer did not complete Ping within 30 seconds; inspect native log')
        time.sleep(0.01)
    return process, log, (time.perf_counter() - start) * 1000


def stop(process, log):
    process.terminate()
    try:
        process.wait(timeout=10)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
        raise RuntimeError('Packaged process did not exit on SIGTERM')
    finally:
        log.close()


def rate(value):
    owner = json.loads((STATE / 'ownership.json').read_text())
    if owner['phase'] != 'ready' or owner['contextB'] != 'kind-kubesift-b':
        raise RuntimeError('Disposable B ownership is not ready')
    expected = [item['uid'] for item in owner['resources'] if item['context'] == 'kind-kubesift-b' and item['kind'] == 'Pod' and item['name'] == 'ks-validation']
    command = ['kubectl', '--kubeconfig', str(STATE / 'kubeconfig'), '--context', 'kind-kubesift-b', '--namespace', 'payments']
    pod = json.loads(subprocess.check_output(command + ['get', 'pod', 'ks-validation', '-o', 'json']))
    if expected != [pod['metadata']['uid']] or pod['metadata']['labels']['kubesift.dev/run'] != owner['runId']:
        raise RuntimeError('Disposable B Pod ownership mismatch')
    if value not in (100, 2000):
        raise RuntimeError('Unapproved fixture rate')
    subprocess.run(command + ['exec', 'ks-validation', '-c', 'fixture', '--', '/bin/sh', '-c', f'printf "{value}\\n" > /tmp/rate'], check=True, timeout=15)


def profile():
    SAMPLES.unlink(missing_ok=True)
    process, log, startup = launch(True)
    print(f'Packaged profile started; pid={process.pid}; frontend Ping={startup:.2f}ms', flush=True)
    start = time.monotonic()
    flood_start = None
    flood_end = None
    done = False
    try:
        with open(STATE / 'native-rss.jsonl', 'w') as rss:
            while time.monotonic() - start < 900:
                elapsed = time.monotonic() - start
                result = subprocess.check_output(['ps', '-o', 'rss=', '-p', str(process.pid)]).decode().strip()
                rss.write(json.dumps({'elapsedSeconds': elapsed, 'nativeRssBytes': int(result) * 1024}) + '\n')
                rss.flush()
                rows = [json.loads(row) for row in SAMPLES.read_text().splitlines()] if SAMPLES.exists() else []
                latest = rows[-1] if rows else None
                loaded = next((row for row in rows if row['ui']['phase'] == 1), None)
                if loaded and flood_start is None and latest['ui']['elapsedMs'] - loaded['ui']['elapsedMs'] >= 120000:
                    rate(2000)
                    flood_start = time.monotonic() - start
                    print('Disposable B flood started at 2000 nominal lines/sec', flush=True)
                if flood_start is not None and flood_end is None and elapsed - flood_start >= 60:
                    rate(100)
                    flood_end = time.monotonic() - start
                    print('Disposable B baseline restored', flush=True)
                if latest and latest['ui']['phase'] in (3, 4):
                    done = latest['ui']['phase'] == 3
                    break
                if process.poll() is not None:
                    raise RuntimeError('Packaged process exited during profile')
                time.sleep(2)
        summary = {'completed': done, 'elapsedSeconds': time.monotonic() - start, 'startupMs': startup, 'floodStartSeconds': flood_start, 'floodEndSeconds': flood_end, 'floodDurationSeconds': None if flood_end is None else flood_end - flood_start}
        (STATE / 'profile-summary.json').write_text(json.dumps(summary, indent=2) + '\n')
        print(json.dumps(summary), flush=True)
        if not done:
            raise RuntimeError('Packaged renderer did not complete the profile; inspect numeric phase-4 evidence and native log')
    finally:
        if flood_start is not None and flood_end is None:
            rate(100)
        stop(process, log)


if __name__ == '__main__':
    if sys.argv[1:] == ['--startup']:
        samples = []
        for index in range(10):
            process, log, elapsed = launch()
            samples.append(elapsed)
            stop(process, log)
            print(f'Start {index + 1}/10: {elapsed:.2f}ms', flush=True)
        (STATE / 'startup-measurements.json').write_text(json.dumps(samples, indent=2) + '\n')
    elif sys.argv[1:] == ['--profile']:
        profile()
    else:
        raise SystemExit('Usage: measure-packaged.py --startup | --profile')
