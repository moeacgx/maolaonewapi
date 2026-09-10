import test from 'node:test';
import assert from 'node:assert/strict';
import {
  canConvertChannelToMultiKey,
  buildChannelKeyUpdateFields,
} from '../multiKeyEditing.js';

test('编辑普通单密钥渠道时显示转换入口', () => {
  assert.equal(
    canConvertChannelToMultiKey({ isEdit: true, originalType: 1, type: 1 }),
    true,
  );
});

test('新建、已有聚合、特殊来源或目标类型、部署托管时不显示转换入口', () => {
  for (const options of [
    { isEdit: false },
    { isMultiKeyChannel: true },
    { originalType: 57 },
    { originalType: 41 },
    { type: 57 },
    { type: 41 },
    { isIonetLocked: true },
  ]) {
    assert.equal(
      canConvertChannelToMultiKey({
        isEdit: true,
        originalType: 1,
        type: 1,
        ...options,
      }),
      false,
    );
  }
});

test('勾选转换时提交显式标记并默认追加和随机', () => {
  assert.deepEqual(buildChannelKeyUpdateFields({ convertToMultiKey: true }), {
    convert_to_multi_key: true,
    key_mode: 'append',
    multi_key_mode: 'random',
  });
});

test('选择覆盖和轮询时保留用户选择', () => {
  assert.deepEqual(
    buildChannelKeyUpdateFields({
      convertToMultiKey: true,
      keyMode: 'replace',
      multiKeyMode: 'polling',
    }),
    {
      convert_to_multi_key: true,
      key_mode: 'replace',
      multi_key_mode: 'polling',
    },
  );
});

test('取消转换后清除残留模式字段', () => {
  const payload = JSON.parse(
    JSON.stringify(
      buildChannelKeyUpdateFields({
        convertToMultiKey: false,
        keyMode: 'replace',
        multiKeyMode: 'polling',
      }),
    ),
  );
  assert.deepEqual(payload, {});
});

test('已有聚合渠道编辑仍提交追加策略但不请求重新转换', () => {
  const payload = JSON.parse(
    JSON.stringify(
      buildChannelKeyUpdateFields({
        isMultiKeyChannel: true,
        multiKeyMode: 'polling',
      }),
    ),
  );
  assert.deepEqual(payload, { key_mode: 'append', multi_key_mode: 'polling' });
});
