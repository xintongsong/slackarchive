#!/usr/bin/env bash
pg_dumpall -U postgres > /backups/$(date "+%Y-%m-%d_%H:%M:%S").bak