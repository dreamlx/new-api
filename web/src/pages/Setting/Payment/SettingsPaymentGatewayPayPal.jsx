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
  Select,
} from '@douyinfe/semi-ui';
const { Text } = Typography;
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsPaymentGatewayPayPal(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    PayPalClientId: '',
    PayPalClientSecret: '',
    PayPalWebhookSecret: '',
    PayPalMode: 'sandbox',
    PayPalMinTopUp: 1,
  });
  const [originInputs, setOriginInputs] = useState({});
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        PayPalClientId: props.options.PayPalClientId || '',
        PayPalClientSecret: props.options.PayPalClientSecret || '',
        PayPalWebhookSecret: props.options.PayPalWebhookSecret || '',
        PayPalMode:
          props.options.PayPalMode !== undefined
            ? props.options.PayPalMode
            : 'sandbox',
        PayPalMinTopUp:
          props.options.PayPalMinTopUp !== undefined
            ? parseFloat(props.options.PayPalMinTopUp)
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

  const submitPayPalSetting = async () => {
    if (props.options.ServerAddress === '') {
      showError(t('请先填写服务器地址'));
      return;
    }

    setLoading(true);
    try {
      const options = [];

      if (inputs.PayPalClientId && inputs.PayPalClientId !== '') {
        options.push({ key: 'PayPalClientId', value: inputs.PayPalClientId });
      }
      if (inputs.PayPalClientSecret && inputs.PayPalClientSecret !== '') {
        options.push({
          key: 'PayPalClientSecret',
          value: inputs.PayPalClientSecret,
        });
      }
      if (inputs.PayPalWebhookSecret && inputs.PayPalWebhookSecret !== '') {
        options.push({
          key: 'PayPalWebhookSecret',
          value: inputs.PayPalWebhookSecret,
        });
      }
      if (
        originInputs['PayPalMode'] !== inputs.PayPalMode &&
        inputs.PayPalMode !== undefined
      ) {
        options.push({
          key: 'PayPalMode',
          value: inputs.PayPalMode,
        });
      }
      if (
        inputs.PayPalMinTopUp !== undefined &&
        inputs.PayPalMinTopUp !== null
      ) {
        options.push({
          key: 'PayPalMinTopUp',
          value: inputs.PayPalMinTopUp.toString(),
        });
      }

      // 发送请求
      const requestQueue = options.map((opt) =>
        API.put('/api/option/', {
          key: opt.key,
          value: opt.value,
        }),
      );

      const results = await Promise.all(requestQueue);

      // 检查所有请求是否成功
      const errorResults = results.filter((res) => !res.data.success);
      if (errorResults.length > 0) {
        errorResults.forEach((res) => {
          showError(res.data.message);
        });
      } else {
        showSuccess(t('更新成功'));
        // 更新本地存储的原始值
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
        <Form.Section text={t('PayPal 设置')}>
          <Text>
            PayPal 凭证请前往
            <a
              href='https://developer.paypal.com/dashboard'
              target='_blank'
              rel='noreferrer'
            >
              PayPal Developer Dashboard
            </a>
            获取，推荐先在
            <a
              href='https://sandbox.paypal.com'
              target='_blank'
              rel='noreferrer'
            >
              Sandbox 环境
            </a>
            测试。
            <br />
          </Text>
          <Banner
            type='info'
            description={`Webhook 地址：${props.options.ServerAddress ? removeTrailingSlash(props.options.ServerAddress) : t('网站地址')}/api/paypal/webhook`}
          />
          <Banner
            type='warning'
            description={t('需要订阅的事件类型：CHECKOUT.ORDER.APPROVED, PAYMENT.CAPTURE.COMPLETED')}
          />
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.Input
                field='PayPalClientId'
                label={t('Client ID')}
                placeholder={t('PayPal 应用的 Client ID，敏感信息不显示')}
                type='password'
              />
            </Col>
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.Input
                field='PayPalClientSecret'
                label={t('Client Secret')}
                placeholder={t('PayPal 应用的 Client Secret，敏感信息不显示')}
                type='password'
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.Input
                field='PayPalWebhookSecret'
                label={t('Webhook Secret')}
                placeholder={t('PayPal Webhook 签名密钥，敏感信息不显示')}
                type='password'
              />
            </Col>
            <Col xs={24} sm={24} md={12} lg={12} xl={12}>
              <Form.Select
                field='PayPalMode'
                label={t('运行模式')}
                style={{ width: '100%' }}
              >
                <Form.Select.Option value='sandbox'>
                  {t('Sandbox (测试环境)')}
                </Form.Select.Option>
                <Form.Select.Option value='live'>
                  {t('Live (生产环境)')}
                </Form.Select.Option>
              </Form.Select>
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='PayPalMinTopUp'
                label={t('最低充值金额')}
                placeholder={t('单位：USD 美元')}
              />
            </Col>
          </Row>
          <Button onClick={submitPayPalSetting}>
            {t('更新 PayPal 设置')}
          </Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
