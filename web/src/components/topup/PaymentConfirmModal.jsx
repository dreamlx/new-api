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

import React from 'react';
import { Modal, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

// PaymentConfirmModal — WeChat Native (QR) top-up modal.
//
// Task 38 ships this as a minimal placeholder: it just displays the raw
// code_url so the recharge flow is reachable end-to-end. Task 39 replaces
// the body with a real QR rendering plus topup-status polling.
const PaymentConfirmModal = ({ visible, codeUrl, tradeNo, onClose }) => {
  const { t } = useTranslation();
  return (
    <Modal
      title={t('微信扫码支付')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      maskClosable={false}
      centered
    >
      <div className='space-y-2'>
        <Text type='secondary'>{t('请使用微信扫描二维码完成支付')}</Text>
        {codeUrl && (
          <div className='break-all text-xs'>
            <Text type='tertiary'>code_url:</Text>
            <div>{codeUrl}</div>
          </div>
        )}
        {tradeNo && (
          <div className='text-xs'>
            <Text type='tertiary'>{t('订单号')}:</Text> {tradeNo}
          </div>
        )}
      </div>
    </Modal>
  );
};

export default PaymentConfirmModal;
