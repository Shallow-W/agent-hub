#!/bin/bash
# Start the Electron desktop shell against an already running backend.
set -e

cd src/frontend
npm run dev:electron
