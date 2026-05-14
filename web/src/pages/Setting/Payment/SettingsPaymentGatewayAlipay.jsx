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

// Heuristic used to detect a masked secret returned from the server. The
// backend masks sensitive option fields as "***xxxx" (asterisks + last 4
// chars) via the GetOptions masking, so we treat any value containing this
// pattern as "not edited" and skip the PUT for that field. Only values that
// the admin re-types are sent upstream.
const isMaskedSecret = (value) =>
  typeof value === 'string' && value.includes('***');

export default function SettingsPaymentGatewayAlipay(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    AlipayEnabled: false,
    AlipayAppId: '',
    AlipayPrivateKey: '',
    AlipayPublicKey: '',
    AlipaySellerId: '',
    AlipayIsSandbox: false,
    AlipayMinTopUp: 1,
  });
  const [originInputs, setOriginInputs] = useState({});
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        AlipayEnabled: props.options.AlipayEnabled === true ||
          props.options.AlipayEnabled === 'true',
        AlipayAppId: props.options.AlipayAppId || '',
        AlipayPrivateKey: props.options.AlipayPrivateKey || '',
        AlipayPublicKey: props.options.AlipayPublicKey || '',
        AlipaySellerId: props.options.AlipaySellerId || '',
        AlipayIsSandbox: props.options.AlipayIsSandbox === true ||
          props.options.AlipayIsSandbox === 'true',
        AlipayMinTopUp:
          props.options.AlipayMinTopUp !== undefined
            ? parseFloat(props.options.AlipayMinTopUp)
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

  const submitAlipaySetting = async () => {
    if (props.options.ServerAddress === '') {
      showError(t('请先填写服务器地址'));
      return;
    }

    setLoading(true);
    try {
      const options = [];

      // AppId — plain field, always sent.
      if (inputs.AlipayAppId !== originInputs.AlipayAppId) {
        options.push({ key: 'AlipayAppId', value: inputs.AlipayAppId });
      }

      // Secret-style fields: only send when the admin has edited them. The
      // backend returns masked previews like "***abcd" via GetOptions, so we
      // must NOT echo those back as the new value.
      if (
        inputs.AlipayPrivateKey &&
        inputs.AlipayPrivateKey !== originInputs.AlipayPrivateKey &&
        !isMaskedSecret(inputs.AlipayPrivateKey)
      ) {
        options.push({
          key: 'AlipayPrivateKey',
          value: inputs.AlipayPrivateKey,
        });
      }

      if (
        inputs.AlipayPublicKey &&
        inputs.AlipayPublicKey !== originInputs.AlipayPublicKey &&
        !isMaskedSecret(inputs.AlipayPublicKey)
      ) {
        options.push({
          key: 'AlipayPublicKey',
          value: inputs.AlipayPublicKey,
        });
      }

      if (inputs.AlipaySellerId !== originInputs.AlipaySellerId) {
        options.push({
          key: 'AlipaySellerId',
          value: inputs.AlipaySellerId || '',
        });
      }

      if (originInputs.AlipayIsSandbox !== inputs.AlipayIsSandbox) {
        options.push({
          key: 'AlipayIsSandbox',
          value: inputs.AlipayIsSandbox ? 'true' : 'false',
        });
      }

      if (
        inputs.AlipayMinTopUp !== undefined &&
        inputs.AlipayMinTopUp !== null &&
        inputs.AlipayMinTopUp !== originInputs.AlipayMinTopUp
      ) {
        options.push({
          key: 'AlipayMinTopUp',
          value: inputs.AlipayMinTopUp.toString(),
        });
      }

      // The Enabled switch is the toggle the admin will flip most often, so
      // always submit it when it differs from the saved value.
      if (originInputs.AlipayEnabled !== inputs.AlipayEnabled) {
        options.push({
          key: 'AlipayEnabled',
          value: inputs.AlipayEnabled ? 'true' : 'false',
        });
      }

      if (options.length === 0) {
        showSuccess(t('更新成功'));
        setLoading(false);
        return;
      }

      const requestQueue = options.map((opt) =>
        API.put('/api/option/', {
          key: opt.key,
          value: opt.value,
        }),
      );

      const results = await Promise.all(requestQueue);
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

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={handleFormChange}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section text={t('支付宝设置')}>
          <Text>
            {t('支付宝应用凭证请前往')}
            <a
              href='https://open.alipay.com/develop/manage'
              target='_blank'
              rel='noreferrer'
            >
              {t('支付宝开放平台')}
            </a>
            {t('获取，建议先在')}
            <a
              href='https://openhome.alipay.com/develop/sandbox/app'
              target='_blank'
              rel='noreferrer'
            >
              {t('沙箱环境')}
            </a>
            {t('进行测试。')}
            <br />
          </Text>
          <Banner
            type='info'
            description={`${t('异步通知地址：')}${props.options.ServerAddress ? removeTrailingSlash(props.options.ServerAddress) : t('网站地址')}/api/user/alipay/notify`}
          />
          <Banner
            type='warning'
            description={t(
              '私钥/公钥为敏感信息，仅在初次配置或需要更换时填写。留空表示沿用已保存的值。',
            )}
          />
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='AlipayAppId'
                label={t('应用 AppId')}
                placeholder={t('支付宝应用的 AppId')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='AlipaySellerId'
                label={t('Seller ID（可选）')}
                placeholder={t('PID，留空表示不校验 seller_id')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='AlipayMinTopUp'
                label={t('最低充值数量')}
                placeholder={t('例如：1')}
                min={1}
                style={{ width: '100%' }}
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.TextArea
                field='AlipayPrivateKey'
                label={t('应用私钥')}
                placeholder={t(
                  '应用私钥 PEM 内容，敏感信息，已保存时此处显示掩码',
                )}
                type='password'
                rows={6}
              />
            </Col>
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.TextArea
                field='AlipayPublicKey'
                label={t('支付宝公钥')}
                placeholder={t(
                  '支付宝公钥 PEM 内容，敏感信息，已保存时此处显示掩码',
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
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='AlipayEnabled'
                label={t('启用支付宝支付')}
                extraText={t('开启后用户充值页将显示支付宝按钮')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='AlipayIsSandbox'
                label={t('沙箱模式')}
                extraText={t('开启后将使用支付宝沙箱网关')}
              />
            </Col>
          </Row>
          <Button onClick={submitAlipaySetting} style={{ marginTop: 16 }}>
            {t('更新支付宝设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
