import building from 'lucide-static/icons/building-2.svg?raw';
import calendarDays from 'lucide-static/icons/calendar-days.svg?raw';
import circleCheck from 'lucide-static/icons/circle-check.svg?raw';
import circleX from 'lucide-static/icons/circle-x.svg?raw';
import columns from 'lucide-static/icons/columns-3.svg?raw';
import eye from 'lucide-static/icons/eye.svg?raw';
import filePlus from 'lucide-static/icons/file-plus.svg?raw';
import fileType from 'lucide-static/icons/file-type.svg?raw';
import files from 'lucide-static/icons/files.svg?raw';
import filter from 'lucide-static/icons/filter.svg?raw';
import history from 'lucide-static/icons/history.svg?raw';
import info from 'lucide-static/icons/info.svg?raw';
import keyboard from 'lucide-static/icons/keyboard.svg?raw';
import landmark from 'lucide-static/icons/landmark.svg?raw';
import link from 'lucide-static/icons/link.svg?raw';
import listOrdered from 'lucide-static/icons/list-ordered.svg?raw';
import mapPin from 'lucide-static/icons/map-pin.svg?raw';
import rotateCcw from 'lucide-static/icons/rotate-ccw.svg?raw';
import route from 'lucide-static/icons/route.svg?raw';
import scale from 'lucide-static/icons/scale.svg?raw';
import searchX from 'lucide-static/icons/search-x.svg?raw';
import shapes from 'lucide-static/icons/shapes.svg?raw';
import tags from 'lucide-static/icons/tags.svg?raw';
import user from 'lucide-static/icons/user.svg?raw';

const docs: Record<string, string> = {
  'docs-body': landmark,
  'docs-category': shapes,
  'docs-columns': columns,
  'docs-day': calendarDays,
  'docs-district': mapPin,
  'docs-failed': circleX,
  'docs-filter': filter,
  'docs-files': files,
  'docs-history': history,
  'docs-info': info,
  'docs-keys': keyboard,
  'docs-link': link,
  'docs-new': filePlus,
  'docs-none': searchX,
  'docs-org': building,
  'docs-passed': circleCheck,
  'docs-person': user,
  'docs-reset': rotateCcw,
  'docs-route': route,
  'docs-show': eye,
  'docs-top': listOrdered,
  'docs-topics': tags,
  'docs-type': fileType,
  'docs-vote': scale,
};

export default docs;
