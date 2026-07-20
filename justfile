#!/usr/bin/env just --justfile
# agent-stream-dbg - Development Commands

# Testing and code quality checks.
[doc('Testing and code quality checks')]
mod test '.justfiles/test.just'

# Tagging and publishing GitHub releases.
[doc('Release (tag and publish)')]
mod release '.justfiles/release.just'

# Live TUI recording integration suite.
[doc('Live TUI recording integration')]
mod vhs '.justfiles/vhs.just'

# Flat development commands.
import '.justfiles/dev.just'

[private]
default:
    @just --list --unsorted
