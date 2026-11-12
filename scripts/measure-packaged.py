#!/usr/bin/env python3
"""
Purpose: Measure actual packaged renderer starts and the approved local load profile.
Used by: Phase 1A packaged acceptance evidence collection.
Responsibilities: Launch the app with a sanitized environment, read frontend-written numeric evidence and sample native RSS.
Important: The only cluster write is a fixed rate change on the journal-owned disposable B fixture; never stop a cluster or use ambient kubeconfig.
"""
import concurrent.futures
import datetime
import json
import os
from pathlib import Path
import socket
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


def source_rate(context, target):
    started = time.monotonic()
    command = ['kubectl', '--kubeconfig', str(STATE / 'kubeconfig'), '--context', context, '--namespace', 'payments', 'logs', 'ks-validation', '-c', 'fixture', '--timestamps=true', '--since=1s', '--follow=true']
    process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        output, errors = process.communicate(timeout=60)
        raise RuntimeError('Source log follow ended before the calibration window')
    except subprocess.TimeoutExpired:
        process.terminate()
        output, errors = process.communicate(timeout=5)
    parts = output.split('\n')
    partial_bytes = len(parts[-1].encode())
    lines = parts[:-1]
    payloads = [line.split(' ', 1) for line in lines]
    timestamps = [datetime.datetime.fromisoformat(line[0].replace('Z', '+00:00')).timestamp() for line in payloads]
    span = timestamps[-1] - timestamps[0]
    window_start = timestamps[0] + 1
    window_seconds = 55
    selected = [line for line, timestamp in zip(payloads, timestamps) if window_start <= timestamp < window_start + window_seconds]
    actual = len(selected) / window_seconds
    lengths = sorted({len(line[1].encode()) + 1 for line in selected})
    result = {'context': context, 'collectionSeconds': time.monotonic() - started, 'collectedUnixSeconds': time.time(), 'targetLinesPerSecond': target, 'lines': len(lines), 'spanSeconds': span, 'windowSeconds': window_seconds, 'windowLines': len(selected), 'discardedPartialBytes': partial_bytes, 'actualLinesPerSecond': actual, 'payloadLengths': lengths, 'toleranceFraction': 0.02, 'pass': span >= 57 and abs(actual / target - 1) <= 0.02 and lengths == [256]}
    with open(STATE / 'source-rates.jsonl', 'a') as stream:
        stream.write(json.dumps(result) + '\n')
    if not result['pass']:
        raise RuntimeError('Actual CRI source rate outside predeclared tolerance; inspect source-rates.jsonl')


def profile():
    SAMPLES.unlink(missing_ok=True)
    (STATE / 'source-rates.jsonl').unlink(missing_ok=True)
    checks = []
    checked_cycles = set()
    baseline_checked = False
    flood_checked = False
    probes = concurrent.futures.ThreadPoolExecutor(max_workers=2)
    pending_probes = []
    flood_probe = None
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
                for row in rows:
                    if row['ui']['phase'] != 2:
                        continue
                    cycle = row['ui']['cycle']
                    if cycle not in checked_cycles:
                        checked_cycles.add(cycle)
                        with socket.socket() as listener:
                            listener.settimeout(0.2)
                            reachable = listener.connect_ex(('127.0.0.1', row['ui']['forwardPort'])) == 0
                        checks.append({'cycle': cycle, 'forwardPort': row['ui']['forwardPort'], 'reachableAfterStop': reachable})
                        (STATE / 'listener-checks.json').write_text(json.dumps(checks, indent=2) + '\n')
                        if reachable:
                            raise RuntimeError('A stopped-cycle forward listener remains reachable')
                for probe in pending_probes:
                    if probe.done():
                        probe.result()
                if loaded and not baseline_checked and elapsed >= 30:
                    pending_probes.extend(probes.submit(source_rate, context, 100) for context in ('kind-dev-local', 'kind-kubesift-b'))
                    baseline_checked = True
                if flood_start is not None and flood_probe is None and elapsed - flood_start >= 2:
                    flood_probe = probes.submit(source_rate, 'kind-kubesift-b', 2000)
                    pending_probes.append(flood_probe)
                if flood_probe is not None and flood_probe.done():
                    flood_probe.result()
                    flood_checked = True
                if loaded and flood_start is None and latest['ui']['elapsedMs'] - loaded['ui']['elapsedMs'] >= 120000:
                    rate(2000)
                    flood_start = time.monotonic() - start
                    print('Disposable B flood started at 2000 paced lines/sec', flush=True)
                if flood_start is not None and flood_end is None and elapsed - flood_start >= 60 and flood_checked:
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
        try:
            if flood_start is not None and flood_end is None:
                rate(100)
        finally:
            try:
                stop(process, log)
            finally:
                probes.shutdown(wait=True)


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
