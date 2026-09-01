/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useState } from 'react';
import { Button, Modal, Toast } from '@douyinfe/semi-ui';
import { Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers';
import { benefitActivityDeleteSkipReasonLabel } from '../../benefits/benefitLabels';

// Selection toolbar for the activities table's batch-delete flow. Deleting
// only ever soft-deletes the *activity* row (shares/vouchers/ledger stay
// intact for audit); the server re-validates every id and may skip some
// (e.g. a row that became active again between selection and submit), so
// the result is always surfaced explicitly rather than a bare "success".
export default function BenefitActivityBatchActions({
  selectedIds,
  onDeleted,
}) {
  const { t } = useTranslation();
  const [deleting, setDeleting] = useState(false);

  const confirmDelete = () => {
    if (selectedIds.length === 0) {
      Toast.error(t('Select at least one activity first'));
      return;
    }
    Modal.confirm({
      title: t('Delete the selected activities?'),
      content: t(
        'Only draft, ended, or terminated activities can be deleted. Shares, vouchers, and ledger entries are kept for audit. This cannot be undone.',
      ),
      onOk: async () => {
        setDeleting(true);
        try {
          const response = await API.delete(
            '/api/benefit/admin/activities/batch',
            { data: { ids: selectedIds } },
          );
          if (!response.data?.success) {
            Toast.error(response.data?.message || t('Batch delete failed'));
            return;
          }
          const result = response.data?.data || {};
          const deletedIds = result.deleted_ids || [];
          const skipped = result.skipped || [];
          Toast.success(
            t('Deleted {{count}} activity(ies)', { count: deletedIds.length }),
          );
          if (skipped.length > 0) {
            Modal.info({
              title: t('Some activities were skipped'),
              content: (
                <ul className='grid gap-1 text-sm'>
                  {skipped.map((entry) => (
                    <li key={entry.id}>
                      #{entry.id}:{' '}
                      {benefitActivityDeleteSkipReasonLabel(t, entry.reason)}
                    </li>
                  ))}
                </ul>
              ),
            });
          }
          onDeleted(deletedIds);
        } catch (error) {
          Toast.error(
            error?.response?.data?.message ||
              error?.message ||
              t('Batch delete failed'),
          );
        } finally {
          setDeleting(false);
        }
      },
    });
  };

  return (
    <Button
      theme='light'
      type='danger'
      icon={<Trash2 size={14} />}
      disabled={selectedIds.length === 0}
      loading={deleting}
      onClick={confirmDelete}
    >
      {t('Delete history')} ({selectedIds.length})
    </Button>
  );
}
