import arrowRight from 'lucide-static/icons/arrow-right.svg?raw';
import calendarDays from 'lucide-static/icons/calendar-days.svg?raw';
import files from 'lucide-static/icons/files.svg?raw';
import landmark from 'lucide-static/icons/landmark.svg?raw';
import layers from 'lucide-static/icons/layers.svg?raw';
import mic from 'lucide-static/icons/mic.svg?raw';
import searchX from 'lucide-static/icons/search-x.svg?raw';
import sliders from 'lucide-static/icons/sliders-horizontal.svg?raw';
import textSearch from 'lucide-static/icons/text-search.svg?raw';
import userRound from 'lucide-static/icons/user-round.svg?raw';
import vote from 'lucide-static/icons/vote.svg?raw';

const suche: Record<string, string> = {
  'suche-all': layers,
  'suche-docs': files,
  'suche-meetings': calendarDays,
  'suche-people': userRound,
  'suche-bodies': landmark,
  'suche-speeches': mic,
  'suche-votes': vote,
  'suche-more': arrowRight,
  'suche-none': searchX,
  'suche-filters': sliders,
  'suche-open': textSearch,
};

export default suche;
