import assert from 'node:assert/strict';
import test from 'node:test';
import {
  MAX_ERROR_MESSAGE_MATCHES_PER_RULE,
  parseErrorMessageReplacementRules,
  serializeErrorMessageReplacementRules,
  validateErrorMessageReplacementRules,
} from './error-message-rules.js';

test('旧 match 会升级为单个匹配值', () => {
  const rules = parseErrorMessageReplacementRules(
    '[{"match":"balance","mode":"contains","replace":"try later"}]',
  );

  assert.deepEqual(rules[0].matches, ['balance']);
  assert.equal(validateErrorMessageReplacementRules(rules), true);
});

test('多匹配值往返时同步保留首个旧字段', () => {
  const rules = parseErrorMessageReplacementRules(
    '[{"match":"legacy","matches":["balance","quota"],"mode":"contains","replace":"try later"}]',
  );

  assert.deepEqual(rules[0].matches, ['balance', 'quota']);
  assert.equal(
    serializeErrorMessageReplacementRules(rules),
    '[{"match":"balance","matches":["balance","quota"],"mode":"contains","replace":"try later"}]',
  );
});

test('拒绝空白和超过上限的匹配值', () => {
  const baseRule = { mode: 'contains', replace: 'try later' };

  assert.equal(
    validateErrorMessageReplacementRules([{ ...baseRule, matches: [] }]),
    false,
  );
  assert.equal(
    validateErrorMessageReplacementRules([
      { ...baseRule, matches: ['balance', ' '] },
    ]),
    false,
  );
  assert.equal(
    validateErrorMessageReplacementRules([
      {
        ...baseRule,
        matches: Array.from(
          { length: MAX_ERROR_MESSAGE_MATCHES_PER_RULE + 1 },
          (_, index) => `match-${index}`,
        ),
      },
    ]),
    false,
  );
});

test('不会静默保留部分无效的匹配数组', () => {
  const rules = parseErrorMessageReplacementRules(
    '[{"matches":["balance",42],"mode":"contains","replace":"try later"}]',
  );

  assert.deepEqual(rules[0].matches, []);
  assert.equal(validateErrorMessageReplacementRules(rules), false);
});
