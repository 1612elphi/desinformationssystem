import { modifier } from 'ember-modifier';
import { draw } from 'ui/modifiers/rough';
import './afd-note.css';

const CHANCE = 0.1;
let mounted = 0;
let rolled = false;

const ARROW = `<svg viewBox="0 0 30 26" aria-hidden="true">
  <path d="M28 5 C 17 3, 8 9, 4 22 M4 22 L 2 14 M4 22 L 11 18" />
</svg>`;

const note = (el: HTMLElement) => {
  // positions the parent
  const undo = draw(el, 'circle', { padding: 6 });
  const label = document.createElement('span');
  label.className = 'dis-afd-note';
  label.innerHTML = `${ARROW}<span>gesichert rechtsextrem</span>`;
  label.style.left = `${el.offsetLeft + el.offsetWidth + 4}px`;
  label.style.top = `${el.offsetTop - 26}px`;
  el.parentElement?.append(label);
  return () => {
    label.remove();
    undo?.();
  };
};

/**
 * `{{afdNote code}}`: one roll per AfD appearance; on a hit, the first AfD
 * chip gets a circle and a labelled arrow.
 */
// ponytail: the roll resets only once every AfD chip has unmounted, so a route change that keeps one mounted keeps the old result
export default modifier((el: HTMLElement, [code]: [string | undefined]) => {
  if (code !== 'AFD') return;
  mounted++;
  const hit = !rolled && Math.random() < CHANCE;
  rolled = true;
  const teardown = hit ? note(el) : undefined;
  return () => {
    teardown?.();
    if (--mounted === 0) rolled = false;
  };
});
