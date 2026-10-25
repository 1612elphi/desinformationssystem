import { module, test } from 'qunit';
import type { Meeting } from 'ui/api';
import { contested, relDate, weekDays } from 'ui/utils/api-dokumente';

module('Unit | Utility | api-dokumente', function () {
  test('relDate picks the unit by distance', function (assert) {
    const today = new Date(2026, 9, 25);
    assert.strictEqual(relDate('2026-10-25', today), 'heute');
    assert.strictEqual(relDate('2026-10-22', today), 'vor 3 Tagen');
    assert.strictEqual(relDate('2026-10-11', today), 'vor 2 Wochen');
    assert.strictEqual(relDate('2026-11-03', today), 'nächste Woche');
    assert.strictEqual(relDate('2024-10-01', today), 'vor 2 Jahren');
    assert.strictEqual(relDate('', today), '');
  });

  test('weekDays runs Monday to Sunday', function (assert) {
    assert.deepEqual(weekDays(new Date(2026, 9, 25)), [
      '2026-10-19',
      '2026-10-20',
      '2026-10-21',
      '2026-10-22',
      '2026-10-23',
      '2026-10-24',
      '2026-10-25',
    ]);
    assert.strictEqual(weekDays(new Date(2026, 9, 19))[0], '2026-10-19');
  });

  test('contested keeps split votes, closest first', function (assert) {
    const m: Meeting = {
      id: 'termin-1',
      agenda_items: [{ anchor: 'top2', title: 'Radweg' }],
      votes: [
        { id: 'a', ja: 40, nein: 0, enthaltung: 4 },
        { id: 'b', ja: 30, nein: 14, enthaltung: 0 },
        { id: 'c', ja: 21, nein: 22, enthaltung: 1, agenda_anchor: 'top2' },
      ],
    };
    const v = contested(m, 4);
    assert.deepEqual(
      v.map((x) => x.id),
      ['c', 'b'],
    );
    assert.strictEqual(v[0]?.title, 'Radweg');
    assert.strictEqual(v[0]?.meeting_id, 'termin-1');
  });
});
