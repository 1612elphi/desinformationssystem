import { modifier } from 'ember-modifier';

export const typing = (t: EventTarget | null) =>
  t instanceof HTMLElement &&
  (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));

export const topDialog = () => {
  const open = [
    ...document.querySelectorAll<HTMLDialogElement>('dialog[open]'),
  ];
  return open.at(-1) ?? null;
};

/**
 * Binds a key while the element is in the DOM: `{{shortcut "j" this.next}}`.
 * Keys follow KeyboardEvent.key ("j", "ArrowLeft", "/", "?"); prefix "mod+"
 * for Cmd on macOS and Ctrl elsewhere. Inside a dialog the key only fires
 * while that dialog is the topmost one; outside dialogs only while none is open.
 */
export default modifier(
  (el: HTMLElement, [key, fn]: [string, (e: KeyboardEvent) => void]) => {
    const mod = key.startsWith('mod+');
    const want = mod ? key.slice(4) : key;
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.altKey) return;
      if ((e.metaKey || e.ctrlKey) !== mod) return;
      if (e.key.toLowerCase() !== want.toLowerCase() && e.key !== want) return;
      if (!mod && typing(e.target)) return;
      const top = topDialog();
      const own = el.closest('dialog');
      if (top ? own !== top : own !== null) return;
      const pane = el.closest('.dis-pane');
      if (pane && !pane.classList.contains('dis-pane--active')) return;
      e.preventDefault();
      fn(e);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  },
);
