#!/usr/bin/env python3
"""Compatibility wrapper for the original prototype server command."""
from pathlib import Path
import os
import sys

script = Path(__file__).resolve().with_name('browser-serve')
os.execv(sys.executable, [sys.executable, str(script), '--port', sys.argv[1] if len(sys.argv) > 1 else '8766'])
