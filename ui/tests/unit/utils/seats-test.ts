import { module, test } from 'qunit';
import { arc } from 'ui/utils/seats';

module('Unit | Utility | seats', function () {
  test('places every seat inside the half circle, left to right', function (assert) {
    for (const n of [0, 1, 2, 5, 11, 47, 48, 120]) {
      const seats = arc(n);
      assert.strictEqual(seats.length, n, `${n} seats`);
      const angles = seats.map((p) => Math.atan2(-p.y, p.x));
      assert.true(
        angles.every((a, i) => i === 0 || a <= angles[i - 1]! + 1e-9),
        `${n}: ordered left to right`,
      );
      assert.true(
        seats.every((p) => Math.hypot(p.x, p.y) <= 1.0001 && p.y <= 1e-9),
        `${n}: inside the upper half circle`,
      );
    }
  });
});
