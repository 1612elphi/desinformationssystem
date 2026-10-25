import archive from 'lucide-static/icons/archive.svg?raw';
import ban from 'lucide-static/icons/ban.svg?raw';
import bookOpen from 'lucide-static/icons/book-open.svg?raw';
import bot from 'lucide-static/icons/bot.svg?raw';
import calendarClock from 'lucide-static/icons/calendar-clock.svg?raw';
import calendarPlus from 'lucide-static/icons/calendar-plus.svg?raw';
import checkCheck from 'lucide-static/icons/check-check.svg?raw';
import cloud from 'lucide-static/icons/cloud.svg?raw';
import fileX from 'lucide-static/icons/file-x.svg?raw';
import files from 'lucide-static/icons/files.svg?raw';
import gavel from 'lucide-static/icons/gavel.svg?raw';
import gitFork from 'lucide-static/icons/git-fork.svg?raw';
import globe from 'lucide-static/icons/globe.svg?raw';
import house from 'lucide-static/icons/house.svg?raw';
import landmark from 'lucide-static/icons/landmark.svg?raw';
import listOrdered from 'lucide-static/icons/list-ordered.svg?raw';
import locateFixed from 'lucide-static/icons/locate-fixed.svg?raw';
import lock from 'lucide-static/icons/lock.svg?raw';
import mapPin from 'lucide-static/icons/map-pin.svg?raw';
import megaphone from 'lucide-static/icons/megaphone.svg?raw';
import messageSquareQuote from 'lucide-static/icons/message-square-quote.svg?raw';
import messagesSquare from 'lucide-static/icons/messages-square.svg?raw';
import scale from 'lucide-static/icons/scale.svg?raw';
import sparkles from 'lucide-static/icons/sparkles.svg?raw';
import thermometer from 'lucide-static/icons/thermometer.svg?raw';
import thumbsDown from 'lucide-static/icons/thumbs-down.svg?raw';
import thumbsUp from 'lucide-static/icons/thumbs-up.svg?raw';
import timer from 'lucide-static/icons/timer.svg?raw';
import usersRound from 'lucide-static/icons/users-round.svg?raw';
import vote from 'lucide-static/icons/vote.svg?raw';

const sitzungen: Record<string, string> = {
  archive,
  ban,
  'book-open': bookOpen,
  bot,
  'calendar-clock': calendarClock,
  'calendar-plus': calendarPlus,
  'check-check': checkCheck,
  cloud,
  'file-x': fileX,
  files,
  gavel,
  'git-fork': gitFork,
  globe,
  house,
  landmark,
  'list-ordered': listOrdered,
  'locate-fixed': locateFixed,
  lock,
  'map-pin': mapPin,
  megaphone,
  'message-square-quote': messageSquareQuote,
  'messages-square': messagesSquare,
  scale,
  sparkles,
  thermometer,
  'thumbs-down': thumbsDown,
  'thumbs-up': thumbsUp,
  timer,
  'users-round': usersRound,
  vote,
};

export default sitzungen;
