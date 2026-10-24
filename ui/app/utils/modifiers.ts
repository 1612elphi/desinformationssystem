import { modifier } from 'ember-modifier';

// Modal behaviour: focus the dialog, close on Escape, lock body scroll.
export const dialog = modifier((el: HTMLElement, [close]: [() => void]) => {
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'Escape') close();
  };
  el.focus();
  document.addEventListener('keydown', onKey);
  document.body.classList.add('cds--body--with-modal-open');
  return () => {
    document.removeEventListener('keydown', onKey);
    document.body.classList.remove('cds--body--with-modal-open');
  };
});

export const clickOutside = modifier((el: HTMLElement, [fn]: [() => void]) => {
  const onClick = (e: MouseEvent) => {
    if (!el.contains(e.target as Node)) fn();
  };
  document.addEventListener('click', onClick);
  return () => document.removeEventListener('click', onClick);
});
