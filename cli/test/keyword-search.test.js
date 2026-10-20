import assert from 'node:assert/strict';
import { test } from 'node:test';
import { keywordPlan } from '../src/keyword-search.js';

test('request classifiers do not obscure the subject', () => {
  for (const query of ['找一个猫表情包', '找一只猫表情包', '请帮我找一个猫表情包', '找一张猫表情包', '找个猫表情包']) {
    assert.deepEqual(keywordPlan(query), keywordPlan('猫'), query);
  }
});
test('ordinary classifiers, numbers and object names remain meaningful', () => {
  for (const query of ['一个猫', '一只猫', '一只猫玩偶', '一个人的夜晚', '一只耳', '两个猫', '3只猫', '一只猫头鹰']) {
    assert.deepEqual(keywordPlan('找'+query), keywordPlan(query), query);
  }
});

test('request phrasing does not erase meaningful classifier-led names', () => {
  for (const name of ['一个人的夜晚', '一只耳', '一只猫玩偶', '两个猫', '3只猫']) {
    assert.deepEqual(keywordPlan('找'+name+'表情包'), keywordPlan(name), name);
  }
});
