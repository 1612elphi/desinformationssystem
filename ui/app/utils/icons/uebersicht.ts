import arrowRight from 'lucide-static/icons/arrow-right.svg?raw';
import calendarCheck from 'lucide-static/icons/calendar-check.svg?raw';
import calendarClock from 'lucide-static/icons/calendar-clock.svg?raw';
import calendarPlus from 'lucide-static/icons/calendar-plus.svg?raw';
import calendarRange from 'lucide-static/icons/calendar-range.svg?raw';
import chartColumn from 'lucide-static/icons/chart-column.svg?raw';
import filePlus from 'lucide-static/icons/file-plus.svg?raw';
import files from 'lucide-static/icons/files.svg?raw';
import gavel from 'lucide-static/icons/gavel.svg?raw';
import hourglass from 'lucide-static/icons/hourglass.svg?raw';
import landmark from 'lucide-static/icons/landmark.svg?raw';
import listOrdered from 'lucide-static/icons/list-ordered.svg?raw';
import map from 'lucide-static/icons/map.svg?raw';
import mapPin from 'lucide-static/icons/map-pin.svg?raw';
import megaphone from 'lucide-static/icons/megaphone.svg?raw';
import messageSquare from 'lucide-static/icons/message-square.svg?raw';
import mic from 'lucide-static/icons/mic.svg?raw';
import refreshCw from 'lucide-static/icons/refresh-cw.svg?raw';
import scale from 'lucide-static/icons/scale.svg?raw';
import scanText from 'lucide-static/icons/scan-text.svg?raw';
import search from 'lucide-static/icons/search.svg?raw';
import sparkles from 'lucide-static/icons/sparkles.svg?raw';
import zap from 'lucide-static/icons/zap.svg?raw';

const uebersicht: Record<string, string> = {
  'dash-agenda': listOrdered,
  'dash-analysed': sparkles,
  'dash-body': landmark,
  'dash-calendar': calendarRange,
  'dash-contested': zap,
  'dash-docs': files,
  'dash-go': arrowRight,
  'dash-ics': calendarPlus,
  'dash-last': calendarCheck,
  'dash-later': hourglass,
  'dash-map': map,
  'dash-meeting': gavel,
  'dash-new': filePlus,
  'dash-next': calendarClock,
  'dash-party': megaphone,
  'dash-place': mapPin,
  'dash-scrape': refreshCw,
  'dash-search': search,
  'dash-speech': messageSquare,
  'dash-speakers': mic,
  'dash-stats': chartColumn,
  'dash-text': scanText,
  'dash-vote': scale,
};

export default uebersicht;
