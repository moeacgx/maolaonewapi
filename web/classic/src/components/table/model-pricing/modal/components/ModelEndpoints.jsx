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

import React, { useEffect, useMemo, useState } from 'react';
import { Button, Tabs, TabPane } from '@douyinfe/semi-ui';
import { ChevronRight, Copy, KeyRound, ScrollText } from 'lucide-react';
import { copy, showSuccess } from '../../../../../helpers';

const LANGUAGES = [
  { key: 'curl', label: 'cURL' },
  { key: 'python', label: 'Python' },
  { key: 'typescript', label: 'TypeScript' },
  { key: 'javascript', label: 'JavaScript' },
];

const getEndpointPath = (type, endpointMap, modelName) => {
  const info = endpointMap[type] || {};
  const path = info.path || '';
  return path.includes('{model}')
    ? path.replaceAll('{model}', modelName || '')
    : path;
};

const getEndpoints = (modelData, endpointMap) => {
  const types = Array.isArray(modelData?.supported_endpoint_types)
    ? modelData.supported_endpoint_types
    : [];
  return types
    .map((type) => ({
      type,
      path: getEndpointPath(type, endpointMap, modelData?.model_name),
      method: endpointMap[type]?.method || 'POST',
    }))
    .filter((endpoint) => endpoint.path);
};

const getBaseUrl = () => {
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin;
  }
  return 'https://api.example.com';
};

const getRequestBody = (endpointType, modelName) => {
  if (endpointType === 'embeddings' || endpointType === 'jina-rerank') {
    return { model: modelName, input: 'The food was delicious.' };
  }
  if (endpointType === 'image-generation') {
    return {
      model: modelName,
      prompt: 'A serene koi pond at sunset.',
      size: '1024x1024',
      n: 1,
    };
  }
  if (endpointType === 'gemini') {
    return {
      contents: [
        { parts: [{ text: 'Explain quantum entanglement in one paragraph.' }] },
      ],
    };
  }
  if (endpointType === 'anthropic') {
    return {
      model: modelName,
      max_tokens: 1024,
      messages: [
        {
          role: 'user',
          content: 'Explain quantum entanglement in one paragraph.',
        },
      ],
    };
  }
  if (endpointType === 'openai-response') {
    return {
      model: modelName,
      input: 'Explain quantum entanglement in one paragraph.',
    };
  }
  return {
    model: modelName,
    messages: [
      {
        role: 'user',
        content: 'Explain quantum entanglement in one paragraph.',
      },
    ],
  };
};

const buildCodeSample = ({ language, endpoint, modelName }) => {
  const baseUrl = getBaseUrl();
  const url = `${baseUrl}${endpoint.path}`;
  const body = JSON.stringify(
    getRequestBody(endpoint.type, modelName),
    null,
    2,
  );
  const isGemini = endpoint.type === 'gemini';
  const isAnthropic = endpoint.type === 'anthropic';
  const authHeader = isAnthropic ? 'x-api-key' : 'Authorization';
  const authValue = isAnthropic ? '$NEW_API_KEY' : 'Bearer $NEW_API_KEY';

  if (language === 'curl') {
    const headers = [
      `  -H '${authHeader}: ${authValue}' \\`,
      ...(isAnthropic ? ["  -H 'anthropic-version: 2023-06-01' \\"] : []),
      `  -H 'Content-Type: application/json' \\`,
    ];
    return [
      `curl '${isGemini ? `${url}?key=$NEW_API_KEY` : url}' \\`,
      ...(isGemini ? headers.slice(1) : headers),
      `  -d '${body.replaceAll('\n', '\n     ')}'`,
    ].join('\n');
  }

  if (language === 'python') {
    if (isAnthropic) {
      return [
        'import anthropic',
        '',
        'client = anthropic.Anthropic(',
        `    base_url='${baseUrl}',`,
        `    api_key='<YOUR_API_KEY>',`,
        ')',
        '',
        'message = client.messages.create(',
        `    model='${modelName}',`,
        '    max_tokens=1024,',
        "    messages=[{'role': 'user', 'content': 'Explain quantum entanglement in one paragraph.'}],",
        ')',
        '',
        'print(message.content[0].text)',
      ].join('\n');
    }
    const headers = isGemini
      ? "{'Content-Type': 'application/json'}"
      : `{\n        '${authHeader}': '${authValue}',\n        'Content-Type': 'application/json',\n    }`;
    return [
      'import requests',
      '',
      `url = '${isGemini ? `${url}?key=$NEW_API_KEY` : url}'`,
      `response = requests.${endpoint.method.toLowerCase()}(`,
      '    url,',
      `    headers=${headers},`,
      `    json=${body.replaceAll('\n', '\n          ')},`,
      ')',
      '',
      'print(response.json())',
    ].join('\n');
  }

  const headers = isGemini
    ? "'Content-Type': 'application/json'"
    : `'${authHeader}': '${authValue}',\n    'Content-Type': 'application/json'`;
  if (isAnthropic) {
    return [
      `const response = await fetch('${url}', {`,
      `  method: '${endpoint.method}',`,
      '  headers: {',
      `    '${authHeader}': '${authValue}',`,
      "    'anthropic-version': '2023-06-01',",
      "    'Content-Type': 'application/json',",
      '  },',
      `  body: JSON.stringify(${body}),`,
      '})',
      '',
      'console.log(await response.json())',
    ].join('\n');
  }
  return [
    `const response = await fetch('${isGemini ? `${url}?key=$NEW_API_KEY` : url}', {`,
    `  method: '${endpoint.method}',`,
    '  headers: {',
    `    ${headers},`,
    '  },',
    `  body: JSON.stringify(${body}),`,
    '})',
    '',
    'console.log(await response.json())',
  ].join('\n');
};

const SectionTitle = ({ icon, children }) => {
  const Icon = icon;
  return (
    <h3 className='classic-pricing-detail-section-title classic-pricing-detail-api-title'>
      <Icon aria-hidden='true' size={14} />
      {children}
    </h3>
  );
};

const CodeSampleSection = ({ modelData, endpoints, t }) => {
  const [endpointType, setEndpointType] = useState(endpoints[0]?.type || '');
  const [language, setLanguage] = useState('curl');

  useEffect(() => {
    setEndpointType(endpoints[0]?.type || '');
    setLanguage('curl');
  }, [modelData?.model_name, endpoints]);

  const activeEndpoint =
    endpoints.find((endpoint) => endpoint.type === endpointType) ||
    endpoints[0];
  const code = useMemo(() => {
    if (!activeEndpoint) return '';
    return buildCodeSample({
      language,
      endpoint: activeEndpoint,
      modelName: modelData?.model_name || '',
    });
  }, [activeEndpoint, language, modelData?.model_name]);

  const handleCopy = async () => {
    if (await copy(code)) showSuccess(t('代码已复制'));
  };

  if (!activeEndpoint) return null;

  return (
    <section className='classic-pricing-detail-api-section'>
      <SectionTitle icon={ScrollText}>{t('代码示例')}</SectionTitle>
      <div className='classic-pricing-detail-api-controls'>
        {endpoints.length > 1 && (
          <Tabs
            activeKey={endpointType}
            type='button'
            onChange={setEndpointType}
          >
            {endpoints.map((endpoint) => (
              <TabPane
                itemKey={endpoint.type}
                key={endpoint.type}
                tab={endpoint.type}
              />
            ))}
          </Tabs>
        )}
        <Tabs
          activeKey={language}
          className='classic-pricing-detail-api-language-tabs'
          type='button'
          onChange={setLanguage}
        >
          {LANGUAGES.map((item) => (
            <TabPane itemKey={item.key} key={item.key} tab={item.label} />
          ))}
        </Tabs>
      </div>
      <div className='classic-pricing-detail-code-block'>
        <pre>
          <code>{code}</code>
        </pre>
        <Button
          aria-label={t('复制代码')}
          className='classic-pricing-detail-code-copy'
          icon={<Copy size={14} />}
          size='small'
          theme='borderless'
          title={t('复制代码')}
          type='tertiary'
          onClick={handleCopy}
        />
      </div>
      <p className='classic-pricing-detail-api-hint'>
        {t('将示例中的 API Key 替换为令牌页面生成的令牌。')}
      </p>
    </section>
  );
};

const AuthSection = ({ t }) => (
  <section className='classic-pricing-detail-api-section'>
    <SectionTitle icon={KeyRound}>{t('身份验证')}</SectionTitle>
    <div className='classic-pricing-detail-auth-box'>
      <ChevronRight aria-hidden='true' size={14} />
      <div>
        <p>
          {t('所有请求都必须包含')}{' '}
          <code>Authorization: Bearer &lt;TOKEN&gt;</code>{' '}
          {t('请求头。Anthropic 端点使用')} <code>x-api-key</code>{' '}
          {t('请求头。')}
        </p>
        <p className='classic-pricing-detail-api-muted'>
          {t('令牌可在令牌页面创建，并可限制模型、分组和访问范围。')}
        </p>
      </div>
    </div>
  </section>
);

const ModelEndpoints = ({ modelData, endpointMap = {}, t }) => {
  const endpoints = useMemo(
    () => getEndpoints(modelData, endpointMap),
    [modelData, endpointMap],
  );

  if (endpoints.length === 0) {
    return (
      <div className='classic-pricing-detail-api-empty'>
        {t('暂无可用 API 端点')}
      </div>
    );
  }

  return (
    <div className='classic-pricing-detail-api-content'>
      <CodeSampleSection modelData={modelData} endpoints={endpoints} t={t} />
      <AuthSection t={t} />
    </div>
  );
};

export default ModelEndpoints;
