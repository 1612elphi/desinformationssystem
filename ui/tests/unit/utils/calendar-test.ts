import { module, test } from 'qunit';
import {
  addMonths,
  bodyKind,
  iso,
  isLive,
  isoWeek,
  lanes,
  monthWeeks,
  parseIso,
  timeWindow,
} from 'ui/utils/calendar';

module('Unit | Utility | calendar', function () {
  test('month weeks start on Monday and cover the month', function (assert) {
    const weeks = monthWeeks(parseIso('2026-10-15'));
    assert.strictEqual(iso(weeks[0]![0]!), '2026-09-28');
    assert.strictEqual(iso(weeks.at(-1)![6]!), '2026-11-01');
    assert.strictEqual(weeks.length, 5);
    assert.strictEqual(monthWeeks(parseIso('2021-02-01')).length, 4);
  });

  test('addMonths clamps to the last day', function (assert) {
    assert.strictEqual(iso(addMonths(parseIso('2026-01-31'), 1)), '2026-02-28');
    assert.strictEqual(
      iso(addMonths(parseIso('2026-03-31'), -1)),
      '2026-02-28',
    );
  });

  test('ISO week numbers', function (assert) {
    assert.strictEqual(isoWeek(parseIso('2026-10-21')), 43);
    assert.strictEqual(isoWeek(parseIso('2021-01-03')), 53);
    assert.strictEqual(isoWeek(parseIso('2024-12-30')), 1);
  });

  test('time windows', function (assert) {
    assert.deepEqual(timeWindow('16.30 bis 18.30 Uhr'), {
      start: 990,
      end: 1110,
    });
    assert.deepEqual(timeWindow('19 bis 21 Uhr'), { start: 1140, end: 1260 });
    assert.strictEqual(timeWindow(undefined), null);
    assert.strictEqual(timeWindow(''), null);
  });

  test('live while now is inside the window', function (assert) {
    const m = { id: 'x', date: '2026-10-21', time: '16.30 bis 18.30 Uhr' };
    assert.true(isLive(m, new Date(2026, 9, 21, 17, 0)));
    assert.false(isLive(m, new Date(2026, 9, 21, 19, 0)));
    assert.false(isLive(m, new Date(2026, 9, 22, 17, 0)));
  });

  test('overlapping windows get separate lanes', function (assert) {
    const w = (x: [number, number]) => ({ start: x[0], end: x[1] });
    const placed = lanes<[number, number]>(
      [
        [990, 1110],
        [990, 1080],
        [1110, 1200],
      ],
      w,
    );
    assert.deepEqual(
      placed.map((p) => p.lane),
      [0, 1, 0],
    );
    assert.true(placed.every((p) => p.lanes === 2));
  });

  test('body kinds', function (assert) {
    assert.strictEqual(bodyKind('Gemeinderat'), 'rat');
    assert.strictEqual(bodyKind('Ortschaftsrat Durlach'), 'ort');
    assert.strictEqual(bodyKind('Hauptausschuss'), 'aus');
    assert.strictEqual(bodyKind('Ausschuss für Umwelt und Gesundheit'), 'aus');
    assert.strictEqual(bodyKind('Migrationsbeirat'), 'other');
  });
});
