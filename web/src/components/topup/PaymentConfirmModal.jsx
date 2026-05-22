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

import React, { useEffect, useRef, useState } from 'react';
import { Modal, Typography, Spin } from '@douyinfe/semi-ui';
import { QRCodeSVG } from 'qrcode.react';
import { CheckCircle, XCircle, AlertTriangle, Clock } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';

const { Text, Title } = Typography;

// Polling interval (milliseconds) for /api/user/topup/status. Tuned to match
// the backend's short-circuit threshold (5s) and balance latency against
// load. Each poll is one DB read; the SDK fallback fires server-side only
// after the order is stale enough to justify it.
const POLL_INTERVAL_MS = 3000;
// Delay before auto-closing the modal after success so the user has time
// to see the success state animation. Matches the spec.
const AUTO_CLOSE_AFTER_SUCCESS_MS = 2000;

// PaymentConfirmModal — WeChat Native (QR) top-up modal.
//
// Lifecycle:
//   - visible flips true => modal mounts, renders QR from code_url, starts
//     polling /api/user/topup/status?trade_no=<tradeNo> every 3 seconds.
//   - Status === "success" => show success state, auto-close after 2s.
//   - Status in {failed, expired, anomaly} => show error state; user
//     dismisses manually.
//   - Modal unmount or visible flips false => polling stops via cleanup.
//
// Network errors during polling are swallowed; we keep retrying until the
// user closes the modal or a terminal status arrives. The backend treats
// /topup/status as cheap (1 DB read, conditional SDK fallback) so this is
// safe.
const PaymentConfirmModal = ({ visible, codeUrl, tradeNo, onClose }) => {
  const { t } = useTranslation();
  // status: 'pending' | 'success' | 'failed' | 'expired' | 'anomaly'
  const [status, setStatus] = useState('pending');
  const cancelledRef = useRef(false);
  const timerRef = useRef(null);
  const autoCloseTimerRef = useRef(null);

  useEffect(() => {
    if (!visible || !tradeNo) {
      return undefined;
    }
    // Reset state when the modal opens for a new order.
    cancelledRef.current = false;
    setStatus('pending');

    const poll = async () => {
      if (cancelledRef.current) return;
      try {
        const res = await API.get('/api/user/topup/status', {
          params: { trade_no: tradeNo },
        });
        const { message, data } = res.data || {};
        if (message === 'success' && data) {
          const next = data.status;
          if (
            next === 'success' ||
            next === 'failed' ||
            next === 'expired' ||
            next === 'anomaly'
          ) {
            if (cancelledRef.current) return;
            setStatus(next);
            if (next === 'success') {
              // Auto-close after a short delay so the success animation is
              // visible; clear in cleanup so a re-open doesn't double-fire.
              autoCloseTimerRef.current = setTimeout(() => {
                if (!cancelledRef.current) onClose?.();
              }, AUTO_CLOSE_AFTER_SUCCESS_MS);
            }
            return;
          }
        }
      } catch (e) {
        // Network errors are transient — keep polling.
      }
      if (!cancelledRef.current) {
        timerRef.current = setTimeout(poll, POLL_INTERVAL_MS);
      }
    };

    timerRef.current = setTimeout(poll, POLL_INTERVAL_MS);

    return () => {
      cancelledRef.current = true;
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      if (autoCloseTimerRef.current) {
        clearTimeout(autoCloseTimerRef.current);
        autoCloseTimerRef.current = null;
      }
    };
    // We intentionally do not include onClose in deps: the consumer may
    // re-create the callback on each render and we'd otherwise restart the
    // poll on every parent re-render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, tradeNo]);

  const renderBody = () => {
    if (status === 'success') {
      return (
        <div className='flex flex-col items-center py-6'>
          <CheckCircle size={64} color='#07C160' />
          <Title heading={5} style={{ marginTop: 12 }}>
            {t('支付成功')}
          </Title>
          <Text type='secondary'>{t('额度已到账，即将关闭...')}</Text>
        </div>
      );
    }
    if (status === 'failed' || status === 'expired' || status === 'anomaly') {
      const isExpired = status === 'expired';
      const isAnomaly = status === 'anomaly';
      const Icon = isExpired ? Clock : isAnomaly ? AlertTriangle : XCircle;
      const color = isExpired ? '#FA8C16' : isAnomaly ? '#FAAD14' : '#F5222D';
      const title = isExpired
        ? t('订单已过期')
        : isAnomaly
          ? t('订单异常')
          : t('支付失败');
      const description = isAnomaly
        ? t('系统检测到金额异常，请联系管理员')
        : isExpired
          ? t('请重新发起充值')
          : t('请稍后重试或联系管理员');
      return (
        <div className='flex flex-col items-center py-6'>
          <Icon size={64} color={color} />
          <Title heading={5} style={{ marginTop: 12 }}>
            {title}
          </Title>
          <Text type='secondary'>{description}</Text>
        </div>
      );
    }
    return (
      <div className='flex flex-col items-center py-2'>
        <Text type='secondary' style={{ marginBottom: 12 }}>
          {t('请使用微信扫描二维码完成支付')}
        </Text>
        {codeUrl ? (
          <div style={{ padding: 12, background: '#fff' }}>
            <QRCodeSVG value={codeUrl} size={200} includeMargin={false} />
          </div>
        ) : (
          <Spin />
        )}
        <div className='flex items-center mt-3'>
          <Spin size='small' />
          <Text type='tertiary' style={{ marginLeft: 8 }}>
            {t('等待支付结果...')}
          </Text>
        </div>
        {tradeNo && (
          <Text
            type='tertiary'
            size='small'
            style={{ marginTop: 8, fontSize: 12 }}
          >
            {t('订单号')}: {tradeNo}
          </Text>
        )}
      </div>
    );
  };

  return (
    <Modal
      title={t('微信扫码支付')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      maskClosable={false}
      centered
      width={380}
    >
      {renderBody()}
    </Modal>
  );
};

export default PaymentConfirmModal;
