import type { TOC } from '@ember/component/template-only';
import './docs-glyph.css';

const GLYPHS: Record<string, [string, string]> = {
  Abstimmungsergebnis: ['AE', 'decision'],
  Beschluss: ['BS', 'decision'],
  Beschlussvorlage: ['BV', 'decision'],
  Vorlage: ['VL', 'decision'],
  Antrag: ['AT', 'motion'],
  Anfrage: ['AF', 'motion'],
  Einladung: ['EL', 'record'],
  Tagesordnung: ['TO', 'record'],
  Protokoll: ['PR', 'record'],
  Niederschrift: ['NS', 'record'],
  Geschäftsordnung: ['GO', 'record'],
  Anlage: ['AL', 'file'],
  Plan: ['PL', 'file'],
  Karte: ['KA', 'file'],
  Präsentation: ['PS', 'file'],
};

const abbr = (t?: string) => GLYPHS[t ?? '']?.[0] ?? (t ?? '?').slice(0, 2);
const group = (t?: string) => GLYPHS[t ?? '']?.[1] ?? 'file';

const DocsGlyph: TOC<{ Element: HTMLSpanElement; Args: { type?: string } }> =
  <template>
    <span
      class="dis-glyph dis-glyph--{{group @type}}"
      title={{@type}}
      aria-hidden="true"
      ...attributes
    >{{abbr @type}}</span>
  </template>;

export default DocsGlyph;
