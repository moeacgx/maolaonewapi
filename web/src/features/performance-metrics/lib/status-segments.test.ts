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
import { describe, expect, test } from "vitest";

import {
  buildStatusSegments,
  getAvailabilityStatusLevel,
  STATUS_SEGMENT_COUNT,
} from "./status-segments";

const END_TS = 24 * 60 * 60;

describe("model availability status segments", () => {
  test("classifies marketplace availability separately from exact success rate", () => {
    expect(getAvailabilityStatusLevel(100)).toBe("healthy");
    expect(getAvailabilityStatusLevel(95)).toBe("healthy");
    expect(getAvailabilityStatusLevel(94.99)).toBe("degraded");
    expect(getAvailabilityStatusLevel(0.01)).toBe("degraded");
    expect(getAvailabilityStatusLevel(0)).toBe("unavailable");
  });

  test("compresses the last 24 hours into fixed six-hour segments", () => {
    const segments = buildStatusSegments(
      [
        { ts: 1 * 60 * 60, success_rate: 100 },
        { ts: 7 * 60 * 60, success_rate: 99.9 },
        { ts: 13 * 60 * 60, success_rate: 99 },
        { ts: 19 * 60 * 60, success_rate: 90 },
      ],
      END_TS,
    );

    expect(segments).toHaveLength(STATUS_SEGMENT_COUNT);
    expect(segments.map((segment) => segment.successRate)).toEqual([
      100, 99.9, 99, 90,
    ]);
    expect(segments.map((segment) => segment.sampleCount)).toEqual([
      1, 1, 1, 1,
    ]);
  });

  test("supports three-signal model cards", () => {
    const segments = buildStatusSegments(
      [
        { ts: 1, success_rate: 100 },
        { ts: 8 * 60 * 60 + 1, success_rate: 75 },
        { ts: 16 * 60 * 60 + 1, success_rate: 0 },
      ],
      END_TS,
      3,
    );

    expect(segments).toHaveLength(3);
    expect(segments.map((segment) => segment.successRate)).toEqual([
      100, 75, 0,
    ]);
  });

  test("averages same-segment buckets and preserves empty segments", () => {
    const segments = buildStatusSegments(
      [
        { ts: 7 * 60 * 60, success_rate: 100 },
        { ts: 8 * 60 * 60, success_rate: 98 },
      ],
      END_TS,
    );

    expect(segments.map((segment) => segment.successRate)).toEqual([
      null,
      99,
      null,
      null,
    ]);
    expect(segments[1].sampleCount).toBe(2);
  });

  test("puts boundary samples in the next segment and includes the end timestamp", () => {
    const segments = buildStatusSegments(
      [
        { ts: 6 * 60 * 60, success_rate: 100 },
        { ts: END_TS, success_rate: 80 },
      ],
      END_TS,
    );

    expect(segments.map((segment) => segment.successRate)).toEqual([
      null,
      100,
      null,
      80,
    ]);
  });

  test("ignores samples outside the window or success-rate bounds", () => {
    const segments = buildStatusSegments(
      [
        { ts: -1, success_rate: 100 },
        { ts: 2 * 60 * 60, success_rate: -1 },
        { ts: 8 * 60 * 60, success_rate: 101 },
        { ts: 14 * 60 * 60, success_rate: Number.NaN },
      ],
      END_TS,
    );

    expect(segments.map((segment) => segment.successRate)).toEqual([
      null,
      null,
      null,
      null,
    ]);
  });
});
