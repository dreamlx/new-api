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

import React, { useEffect, useState, useRef } from 'react';
import {
  Banner,
  Button,
  Form,
  Row,
  Col,
  Typography,
  Spin,
} from '@douyinfe/semi-ui';
const { Text } = Typography;
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

// See SettingsPaymentGatewayAlipay for the rationale. The backend masks
// sensitive option values like "***abcd"; we must not echo those back as
// new values.
const isMaskedSecret = (value) =>
  typeof value === 'string' && value.includes('***');

export default function SettingsPaymentGatewayWxpay(props) {
  const { t } = useTranslation();
  const sectionTitle = props.hideSectionTitle ? undefined : t('微信支付设置');
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    WxpayEnabled: false,
    WxpayAppId: '',
    WxpayMchId: '',
    WxpayMchSerialNo: '',
    WxpayApiV3Key: '',
    WxpayPrivateKey: '',
    WxpayPublicKeyId: '',
    WxpayPublicKey: '',
    WxpayMinTopUp: 1,
  });
  const [originInputs, setOriginInputs] = useState({});
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        WxpayEnabled: props.options.WxpayEnabled === true ||
          props.options.WxpayEnabled === 'true',
        WxpayAppId: props.options.WxpayAppId || '',
        WxpayMchId: props.options.WxpayMchId || '',
        WxpayMchSerialNo: props.options.WxpayMchSerialNo || '',
        WxpayApiV3Key: props.options.WxpayApiV3Key || '',
        WxpayPrivateKey: props.options.WxpayPrivateKey || '',
        WxpayPublicKeyId: props.options.WxpayPublicKeyId || '',
        WxpayPublicKey: props.options.WxpayPublicKey || '',
        WxpayMinTopUp:
          props.options.WxpayMinTopUp !== undefined
            ? parseFloat(props.options.WxpayMinTopUp)
            : 1,
      };
      setInputs(currentInputs);
      setOriginInputs({ ...currentInputs });
      formApiRef.current.setValues(currentInputs);
    }
  }, [props.options]);

  const handleFormChange = (values) => {
    setInputs(values);
  };

  const submitWxpaySetting = async () => {
    if (props.options.ServerAddress === '') {
      showError(t('请先填写服务器地址'));
      return;
    }

    setLoading(true);
    try {
      const options = [];

      // Plain (non-secret) fields — push when changed.
      const plainFields = [
        'WxpayAppId',
        'WxpayMchId',
        'WxpayMchSerialNo',
        'WxpayPublicKeyId',
      ];
      plainFields.forEach((key) => {
        if (inputs[key] !== originInputs[key]) {
          options.push({ key, value: inputs[key] || '' });
        }
      });

      // Secret fields — only when admin actually typed something new.
      if (
        inputs.WxpayApiV3Key &&
        inputs.WxpayApiV3Key !== originInputs.WxpayApiV3Key &&
        !isMaskedSecret(inputs.WxpayApiV3Key)
      ) {
        options.push({
          key: 'WxpayApiV3Key',
          value: inputs.WxpayApiV3Key,
        });
      }

      if (
        inputs.WxpayPrivateKey &&
        inputs.WxpayPrivateKey !== originInputs.WxpayPrivateKey &&
        !isMaskedSecret(inputs.WxpayPrivateKey)
      ) {
        options.push({
          key: 'WxpayPrivateKey',
          value: inputs.WxpayPrivateKey,
        });
      }

      if (
        inputs.WxpayPublicKey &&
        inputs.WxpayPublicKey !== originInputs.WxpayPublicKey &&
        !isMaskedSecret(inputs.WxpayPublicKey)
      ) {
        options.push({
          key: 'WxpayPublicKey',
          value: inputs.WxpayPublicKey,
        });
      }

      if (
        inputs.WxpayMinTopUp !== undefined &&
        inputs.WxpayMinTopUp !== null &&
        inputs.WxpayMinTopUp !== originInputs.WxpayMinTopUp
      ) {
        options.push({
          key: 'WxpayMinTopUp',
          value: inputs.WxpayMinTopUp.toString(),
        });
      }

      let enabledOption = null;
      if (originInputs.WxpayEnabled !== inputs.WxpayEnabled) {
        enabledOption = {
          key: 'WxpayEnabled',
          value: inputs.WxpayEnabled ? 'true' : 'false',
        };
      }

      if (options.length === 0 && !enabledOption) {
        showSuccess(t('更新成功'));
        setLoading(false);
        return;
      }

      const results = [];
      for (const opt of options) {
        results.push(
          await API.put('/api/option/', {
            key: opt.key,
            value: opt.value,
          }),
        );
      }
      if (enabledOption) {
        results.push(
          await API.put('/api/option/', {
            key: enabledOption.key,
            value: enabledOption.value,
          }),
        );
      }
      const errorResults = results.filter((res) => !res.data.success);
      if (errorResults.length > 0) {
        errorResults.forEach((res) => {
          showError(res.data.message);
        });
      } else {
        showSuccess(t('更新成功'));
        setOriginInputs({ ...inputs });
        props.refresh?.();
      }
    } catch (error) {
      showError(t('更新失败'));
    }
    setLoading(false);
  };

  const serverAddress = props.options.ServerAddress
    ? removeTrailingSlash(props.options.ServerAddress)
    : t('网站地址');
  const defaultNotifyURL = `${serverAddress}/api/user/wxpay/notify`;

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={handleFormChange}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section text={sectionTitle}>
          <Text>
            {t('微信支付商户凭证请前往')}
            <a
              href='https://pay.weixin.qq.com/'
              target='_blank'
              rel='noreferrer'
            >
              {t('微信支付商户平台')}
            </a>
            {t('获取，本系统使用 Native（扫码）下单模式。')}
            <br />
          </Text>
          <Banner
            type='info'
            description={`${t('默认异步通知地址：')}${defaultNotifyURL}`}
          />
          <Banner
            type='warning'
            description={t(
              '当前使用微信支付公钥模式。API v3 密钥、商户私钥与微信支付公钥为敏感信息，仅在初次配置或更换时填写。留空表示沿用已保存的值。',
            )}
          />
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='WxpayAppId'
                label={t('AppId')}
                placeholder={t('公众号/小程序/移动应用 AppId')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='WxpayMchId'
                label={t('商户号 MchId')}
                placeholder={t('微信支付商户号')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='WxpayMchSerialNo'
                label={t('商户证书序列号')}
                placeholder={t('商户 API 证书序列号')}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='WxpayPublicKeyId'
                label={t('微信支付公钥 ID')}
                placeholder='PUB_KEY_ID_...'
              />
            </Col>
            <Col xs={24} sm={24} md={16} lg={16} xl={16}>
              <Form.TextArea
                field='WxpayPublicKey'
                label={t('微信支付公钥')}
                placeholder={t(
                  'pub_key.pem 完整内容，需包含 BEGIN/END PUBLIC KEY',
                )}
                type='password'
                rows={4}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.TextArea
                field='WxpayApiV3Key'
                label={t('API v3 密钥')}
                placeholder={t(
                  'API v3 密钥（32 位字符串），敏感信息，已保存时此处显示掩码',
                )}
                type='password'
                rows={3}
              />
            </Col>
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.TextArea
                field='WxpayPrivateKey'
                label={t('商户私钥')}
                placeholder={t(
                  '商户 apiclient_key.pem 内容，敏感信息，已保存时此处显示掩码',
                )}
                type='password'
                rows={6}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.InputNumber
                field='WxpayMinTopUp'
                label={t('最低充值数量')}
                placeholder={t('例如：1')}
                min={1}
                style={{ width: '100%' }}
              />
            </Col>
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.Switch
                field='WxpayEnabled'
                label={t('启用微信支付')}
                extraText={t('开启后用户充值页将显示微信扫码按钮')}
              />
            </Col>
          </Row>
          <Button onClick={submitWxpaySetting} style={{ marginTop: 16 }}>
            {t('更新微信支付设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
