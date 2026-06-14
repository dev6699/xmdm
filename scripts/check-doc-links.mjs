#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const repoRoot = process.cwd();
const markdownFiles = findMarkdownFiles(repoRoot);
const problems = [];

for (const file of markdownFiles) {
  const contents = fs.readFileSync(file, 'utf8');
  checkTrailingWhitespace(file, contents, problems);
  checkLinks(file, contents, problems);
}

if (problems.length > 0) {
  for (const problem of problems) {
    console.error(problem);
  }
  process.exit(1);
}

function findMarkdownFiles(root) {
  const files = [];
  walk(root, files);
  return files.filter((file) => file.endsWith('.md'));
}

function walk(currentPath, files) {
  for (const entry of fs.readdirSync(currentPath, { withFileTypes: true })) {
    if (entry.name === '.git' || entry.name === 'node_modules' || entry.name === '.gradle') {
      continue;
    }
    const fullPath = path.join(currentPath, entry.name);
    if (entry.isDirectory()) {
      walk(fullPath, files);
      continue;
    }
    if (entry.isFile()) {
      files.push(fullPath);
    }
  }
}

function checkTrailingWhitespace(file, contents, problems) {
  const lines = contents.split(/\r?\n/);
  let inFence = false;
  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];
    if (/^\s*```/.test(line)) {
      inFence = !inFence;
      continue;
    }
    if (inFence) {
      continue;
    }
    if (/[ \t]+$/.test(line)) {
      problems.push(`${relative(file)}:${i + 1}: trailing whitespace`);
    }
  }
}

function checkLinks(file, contents, problems) {
  const lines = contents.split(/\r?\n/);
  let inFence = false;
  const linkPattern = /!?\[[^\]]*\]\(([^)]+)\)/g;

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];
    if (/^\s*```/.test(line)) {
      inFence = !inFence;
      continue;
    }
    if (inFence) {
      continue;
    }

    for (const match of line.matchAll(linkPattern)) {
      const rawTarget = match[1].trim();
      if (shouldSkipTarget(rawTarget)) {
        continue;
      }

      const target = stripFragmentAndQuery(rawTarget);
      const resolved = path.resolve(path.dirname(file), target);
      if (!fs.existsSync(resolved)) {
        problems.push(`${relative(file)}:${i + 1}: missing link target ${rawTarget}`);
      }
    }
  }
}

function shouldSkipTarget(target) {
  return (
    target.startsWith('#') ||
    target.startsWith('http://') ||
    target.startsWith('https://') ||
    target.startsWith('mailto:') ||
    target.startsWith('data:') ||
    target.startsWith('javascript:')
  );
}

function stripFragmentAndQuery(target) {
  return target.split('#', 1)[0].split('?', 1)[0];
}

function relative(file) {
  return path.relative(repoRoot, file);
}
