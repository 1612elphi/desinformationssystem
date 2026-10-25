export function arc(n: number): { x: number; y: number }[] {
  if (n <= 0) return [];
  const rows = Math.max(1, Math.min(8, Math.round(Math.sqrt(n / 2.6))));
  const radii = Array.from({ length: rows }, (_, i) =>
    rows === 1 ? 0.8 : 0.45 + (0.55 * i) / (rows - 1),
  );
  const total = radii.reduce((a, r) => a + r, 0);
  const counts = radii.map((r) => Math.floor((n * r) / total));
  // rounding leftovers outside in
  for (
    let left = n - counts.reduce((a, c) => a + c, 0), i = rows - 1;
    left > 0;
    left--, i = (i - 1 + rows) % rows
  )
    counts[i]!++;
  const seats: { x: number; y: number; t: number; r: number }[] = [];
  radii.forEach((r, i) => {
    const c = counts[i]!;
    for (let k = 0; k < c; k++) {
      const t = c === 1 ? Math.PI / 2 : Math.PI - (Math.PI * k) / (c - 1);
      seats.push({ x: r * Math.cos(t), y: -r * Math.sin(t), t, r });
    }
  });
  return seats.sort((a, b) => b.t - a.t || a.r - b.r);
}
