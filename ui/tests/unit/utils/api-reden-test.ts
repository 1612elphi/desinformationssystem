import { module, test } from 'qunit';
import { clock, mins } from 'ui/utils/api-reden';

module('Unit | Utility | api-reden', function () {
  test('formats a stream offset as h:mm:ss', function (assert) {
    assert.strictEqual(clock(0), '0:00:00');
    assert.strictEqual(clock(1609.6), '0:26:49');
    assert.strictEqual(clock(5394.9), '1:29:54');
  });

  test('rounds seconds to minutes', function (assert) {
    assert.strictEqual(mins(89), '1 min');
    assert.strictEqual(mins(6519), '109 min');
  });
});
