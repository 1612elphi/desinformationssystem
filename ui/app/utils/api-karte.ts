import { getJSON } from 'ui/api';

export const KINDS = [
  'street',
  'square',
  'park',
  'building',
  'school',
  'district',
  'area',
  'water',
  'transit',
] as const;
export type Kind = (typeof KINDS)[number];

export const KIND_LABEL: Record<Kind, string> = {
  street: 'Straße',
  square: 'Platz',
  park: 'Park',
  building: 'Gebäude',
  school: 'Schule',
  district: 'Stadtteil',
  area: 'Gebiet',
  water: 'Gewässer',
  transit: 'Verkehr',
};

export interface Place {
  id: number;
  name: string;
  kind: Kind;
  lat: number;
  lon: number;
  docs: number;
  mentions: number;
}

export interface Places {
  places: Place[];
  total_docs: number;
}

export interface PlaceDoc {
  id: string;
  label: string;
  doc_type?: string;
  meeting_id?: string;
  meeting_date?: string;
  agenda_title?: string;
  n: number;
  quote?: string;
}

export interface PlaceDetail extends Omit<Place, 'docs' | 'mentions'> {
  documents: PlaceDoc[];
}

export const fetchPlaces = (from: string) =>
  getJSON<Places>(`/api/places${from ? `?from=${from}` : ''}`);

export const fetchPlace = (id: number) =>
  getJSON<PlaceDetail>(`/api/places/${id}`);
