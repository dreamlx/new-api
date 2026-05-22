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

const WeChatPayIcon = ({ size = 18 }) => {
  const height = size;
  const width = Math.round(size * 1.45);

  return (
    <svg
      width={width}
      height={height}
      viewBox='0 0 58 40'
      role='img'
      aria-label='WeChat Pay'
      xmlns='http://www.w3.org/2000/svg'
    >
      <rect x='0' y='0' width='58' height='40' rx='8' fill='#07C160' />
      <path
        d='M47.6 18.7c0 8.9-7.9 16.2-17.6 16.2-2.5 0-4.8-.5-7-1.3l-5.2 2.5c-.8.4-1.7-.3-1.5-1.2l1-5.2c-3.1-2.9-4.9-6.8-4.9-11 0-9 7.9-16.2 17.6-16.2s17.6 7.2 17.6 16.2Z'
        fill='#00C300'
      />
      <path
        d='M21.8 17.1c-.9-1.7.2-3.1 1.9-1.8l4.1 3c1.2.9 2.4.7 3.7.1l14.1-6.5c1.6-.7 2.7 1.2 1.4 2.3L30.2 27.5c-1.9 1.5-4.2 1-5.1-1.3l-3.3-9.1Z'
        fill='#fff'
      />
    </svg>
  );
};

export default WeChatPayIcon;
