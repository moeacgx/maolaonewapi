import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

import { buildInvoicePaymentRequest } from '../../../components/invoice/paymentRequest.js';

const root = dirname(fileURLToPath(import.meta.url));
const modalSource = readFileSync(
  resolve(root, '../../../components/invoice/InvoiceBatchRequestModal.jsx'),
  'utf8',
);

test('classic invoice fee payment submits the selected configured Epay method', () => {
  assert.match(modalSource, /getConfiguredPaymentMethods\(config\)/);
  assert.match(modalSource, /API\.post\('\/api\/user\/invoice\/payment'/);
  assert.match(modalSource, /buildInvoicePaymentRequest\(/);
  assert.match(
    modalSource,
    /API\.post\('\/api\/user\/invoice\/payment', paymentRequest\)/,
  );
});

test('classic invoice fee payment does not send a pending Epay request through balance endpoint', () => {
  assert.match(
    modalSource,
    /!paymentRequired \|\| balanceSelected\s*\? await API\.post\('\/api\/user\/invoice\/request'/,
  );
  assert.match(
    modalSource,
    /: await API\.post\('\/api\/user\/invoice\/payment'/,
  );
});

test('classic invoice fee payment preserves the selected Epay method in its request payload', () => {
  assert.deepEqual(
    buildInvoicePaymentRequest(
      [{ source_type: 'topup', source_id: 'TOP-1' }],
      { required: true, type: 'personal', kind: 'normal', title: '测试' },
      'alipay',
    ),
    {
      orders: [{ source_type: 'topup', source_id: 'TOP-1' }],
      invoice: {
        required: true,
        type: 'personal',
        kind: 'normal',
        title: '测试',
      },
      payment_method: 'alipay',
    },
  );
});
