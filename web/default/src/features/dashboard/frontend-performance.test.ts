/*
Copyright (C) 2023-2026 QuantumNous

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
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { describe, test } from 'node:test'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const readDefaultSource = (...parts: string[]) =>
  readFileSync(resolve(root, ...parts), 'utf8')
const readClassicSource = (...parts: string[]) =>
  readFileSync(
    resolve(root, '..', '..', '..', '..', 'classic', 'src', ...parts),
    'utf8'
  )

describe('frontend idle performance guards', () => {
  test('Classic sidebar does not restart its route-selection effect on every render', () => {
    const hookSource = readClassicSource('hooks', 'common', 'useSidebar.js')
    const sidebarSource = readClassicSource(
      'components',
      'layout',
      'SiderBar.jsx'
    )

    assert.match(hookSource, /\buseCallback\b/)
    assert.match(
      hookSource,
      /const isModuleVisible = useCallback\([\s\S]*?\[finalConfig\][\s\S]*?\);/
    )
    assert.match(
      sidebarSource,
      /setSelectedKeys\(\(keys\) =>[\s\S]*?keys\.length === 1 && keys\[0\] === matchingKey[\s\S]*?\? keys[\s\S]*?: \[matchingKey\]/
    )
  })

  test('decorative shine effects stop after one entrance and respect reduced motion', () => {
    const classicStyles = readClassicSource('index.css')
    const defaultDashboard = readDefaultSource(
      'components',
      'overview',
      'overview-dashboard.tsx'
    )

    assert.match(classicStyles, /animation:\s*sweep-shine 4s linear 1 forwards/)
    assert.doesNotMatch(
      classicStyles,
      /animation:\s*sweep-shine[^;]*\binfinite\b/
    )
    assert.match(
      classicStyles,
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*?\.shine-text[\s\S]*?animation:\s*none/
    )
    assert.match(
      defaultDashboard,
      /animate=\{\{ x: \['-100%', '100%'\] \}\}[\s\S]*?transition=\{\{ duration: 3\.2, ease: 'easeInOut' \}\}/
    )
    assert.doesNotMatch(defaultDashboard, /repeat:\s*Infinity/)
  })
})
