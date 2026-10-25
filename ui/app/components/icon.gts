import type { TOC } from '@ember/component/template-only';
import { htmlSafe } from '@ember/template';

// every file in utils/icons/ is a name -> lucide SVG map; one file per area avoids merge clashes
const SVGS: Record<string, string> = {};
for (const icons of Object.values(
  import.meta.glob<Record<string, string>>('../utils/icons/*.ts', {
    eager: true,
    import: 'default',
  }),
))
  Object.assign(SVGS, icons);

const svg = (name: string) => htmlSafe(SVGS[name] ?? '');
const size = (px?: number) => htmlSafe(`--dis-icon-size: ${px ?? 16}px`);

const Icon: TOC<{
  Element: HTMLSpanElement;
  Args: { name: string; size?: number };
}> = <template>
  <span
    class="dis-icon"
    style={{size @size}}
    aria-hidden="true"
    ...attributes
  >{{svg @name}}</span>
</template>;

export default Icon;
