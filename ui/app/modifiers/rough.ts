import { modifier } from 'ember-modifier';
import { annotate } from 'rough-notation';

type RoughType =
  | 'underline'
  | 'box'
  | 'circle'
  | 'highlight'
  | 'strike-through'
  | 'crossed-off'
  | 'bracket';

interface Options {
  // a colour token such as "--dis-highlight"; default the primary colour
  color?: string;
  // false draws nothing, so a condition can switch the annotation off
  show?: boolean;
  padding?: number;
  strokeWidth?: number;
  iterations?: number;
  multiline?: boolean;
  brackets?: 'left' | 'right' | 'top' | 'bottom';
  // ms before the drawing starts, to stagger several annotations
  delay?: number;
}

// rough-notation writes the colour into an SVG attribute, where var() does not resolve
const resolve = (token: string) =>
  getComputedStyle(document.documentElement).getPropertyValue(token).trim() ||
  'currentColor';

/**
 * A hand-drawn mark on the element: `{{rough "underline" color="--dis-highlight"}}`.
 * The colour follows the light/dark switch; under reduced motion it appears
 * without the drawing animation.
 */
export default modifier(
  (el: HTMLElement, [type]: [RoughType], options: Options) => {
    if (options.show === false) return;
    // the mark is an SVG sibling; inside a table row it would become a stray cell
    if (/^(TD|TH|TR)$/.test(el.tagName)) {
      console.warn('rough: mark an element inside the cell, not the cell', el);
      return;
    }
    // the SVG is placed absolutely against the parent; a static parent lets it drift on scroll
    const parent = el.parentElement;
    const restore =
      parent && getComputedStyle(parent).position === 'static'
        ? parent.style.position
        : undefined;
    if (parent && restore !== undefined) parent.style.position = 'relative';
    const token = options.color ?? '--canon-primary';
    const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
    const mark = annotate(el, {
      type,
      color: resolve(token),
      animate: !still,
      animationDuration: 700,
      padding: options.padding ?? 4,
      strokeWidth: options.strokeWidth ?? 1.6,
      iterations: options.iterations ?? 2,
      multiline: options.multiline ?? false,
      brackets: options.brackets,
    });
    const timer = setTimeout(() => mark.show(), options.delay ?? 0);
    const scheme = matchMedia('(prefers-color-scheme: dark)');
    const recolour = () => (mark.color = resolve(token));
    scheme.addEventListener('change', recolour);
    return () => {
      clearTimeout(timer);
      scheme.removeEventListener('change', recolour);
      mark.remove();
      if (parent && restore !== undefined) parent.style.position = restore;
    };
  },
);
