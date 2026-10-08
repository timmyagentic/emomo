#!/usr/bin/env node
import { libraryMain } from '../src/library.js';
process.exitCode = await libraryMain(process.argv.slice(2));
