// Lucide icons by the name the app uses; other files in this folder add more.
import calendar from 'lucide-static/icons/calendar.svg?raw';
import chartColumn from 'lucide-static/icons/chart-column.svg?raw';
import check from 'lucide-static/icons/check.svg?raw';
import chevronDown from 'lucide-static/icons/chevron-down.svg?raw';
import chevronLeft from 'lucide-static/icons/chevron-left.svg?raw';
import chevronRight from 'lucide-static/icons/chevron-right.svg?raw';
import clock from 'lucide-static/icons/clock.svg?raw';
import externalLink from 'lucide-static/icons/external-link.svg?raw';
import fileText from 'lucide-static/icons/file-text.svg?raw';
import mic from 'lucide-static/icons/mic.svg?raw';
import map from 'lucide-static/icons/map.svg?raw';
import radio from 'lucide-static/icons/radio.svg?raw';
import refreshCw from 'lucide-static/icons/refresh-cw.svg?raw';
import rss from 'lucide-static/icons/rss.svg?raw';
import search from 'lucide-static/icons/search.svg?raw';
import users from 'lucide-static/icons/users.svg?raw';
import x from 'lucide-static/icons/x.svg?raw';

const base: Record<string, string> = {
  calendar,
  chart: chartColumn,
  checkmark: check,
  'chevron-down': chevronDown,
  'chevron-left': chevronLeft,
  'chevron-right': chevronRight,
  'caret-left': chevronLeft,
  'caret-right': chevronRight,
  close: x,
  document: fileText,
  launch: externalLink,
  live: radio,
  map,
  mic,
  renew: refreshCw,
  rss,
  search,
  time: clock,
  users,
};

export default base;
