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
        d='M21.4 12.2c-6.1 0-11 3.9-11 8.7 0 2.7 1.6 5.1 4 6.7l-.9 3.1 3.6-1.9c1.3.4 2.7.7 4.3.7 6.1 0 11-3.9 11-8.7s-4.9-8.6-11-8.6Zm-3.7 6.4c-.9 0-1.5-.6-1.5-1.4 0-.8.6-1.4 1.5-1.4s1.5.6 1.5 1.4c0 .8-.6 1.4-1.5 1.4Zm7.4 0c-.9 0-1.5-.6-1.5-1.4 0-.8.6-1.4 1.5-1.4s1.5.6 1.5 1.4c0 .8-.6 1.4-1.5 1.4Z'
        fill='#fff'
      />
      <path
        d='M38.7 16.8c-4.9 0-8.9 3.1-8.9 7 0 3.8 4 7 8.9 7 1.2 0 2.4-.2 3.5-.6l3 1.6-.7-2.5c2-1.3 3.2-3.2 3.2-5.4 0-4-4-7.1-9-7.1Zm-3 5.2c-.7 0-1.2-.5-1.2-1.1 0-.6.5-1.1 1.2-1.1.7 0 1.2.5 1.2 1.1 0 .6-.5 1.1-1.2 1.1Zm6 0c-.7 0-1.2-.5-1.2-1.1 0-.6.5-1.1 1.2-1.1.7 0 1.2.5 1.2 1.1 0 .6-.5 1.1-1.2 1.1Z'
        fill='#fff'
        opacity='0.92'
      />
    </svg>
  );
};

export default WeChatPayIcon;
