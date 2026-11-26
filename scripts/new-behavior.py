#!/usr/bin/env python3
"""Accumulates binary checks for the open change; shared loading stays after folding."""
import importlib.util
from pathlib import Path

spec = importlib.util.spec_from_file_location('baseline', Path(__file__).with_name('cli-checks.py'))
baseline = importlib.util.module_from_spec(spec)
spec.loader.exec_module(baseline)


# No open change. Add permanent or inverse cases here before implementation.

if __name__ == '__main__':
    print('No open behavior change.')
