import assert from 'node:assert/strict';
import { test } from 'node:test';
import { keywordPlan } from '../src/keyword-search.js';

test('keyword matching does not initialize locale collation', t => {
  t.mock.method(String.prototype, 'localeCompare', () => {
    throw new Error('Keyword matching must not use locale collation.');
  });
  for (const query of ['猫 无语', '请帮我找一张熊猫头开心的表情包', 'hello kitty']) {
    assert.ok(keywordPlan(query).expression);
  }
  assert.deepEqual(keywordPlan('甲甲甲乙乙', {
    subjects: { 乙乙: ['乙乙'], 甲甲: ['甲甲'], 甲甲甲: ['甲甲甲'] },
  }).groups.map(group => group.key), ['甲甲甲', '乙乙']);
});

test('longest prefixes, equal-length terms and explicit alias keys keep their matches', () => {
  const subjects = { 乙乙: ['乙乙'], 甲甲: ['甲甲'], 甲甲甲: ['甲甲甲'] };
  for (const entries of [Object.entries(subjects), Object.entries(subjects).reverse()]) {
    const plan = keywordPlan('甲甲甲乙乙', { subjects: Object.fromEntries(entries) });
    assert.deepEqual(plan.groups.map(group => group.key), ['甲甲甲', '乙乙']);
    assert.equal(plan.literalExpression, '("甲甲") AND ("乙乙")');
  }
  const plan = keywordPlan('丙丙', {
    intents: { 乙乙: ['丙丙'], 甲甲: ['丙丙'] },
    subjects: { 丙丙: ['丁丁'] },
  });
  assert.deepEqual(plan.groups.map(({ key, terms, subject, intent }) => ({ key, terms, subject, intent })), [
    { key: '丙丙', terms: ['丙丙', '丁丁'], subject: true, intent: false },
  ]);
});
