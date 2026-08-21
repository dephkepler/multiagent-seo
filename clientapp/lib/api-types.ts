// Mirrors the Client schemas in backend/api/openapi.yaml. Hand-written rather
// than generated: it is four shapes, and the generator would drag the whole
// admin API into a bundle that cold-starts on mobile data.

export interface BookingOptions {
  slots: string[]
  categories: string[]
}

export interface Consultation {
  id: string
  scheduled_at: string
  status: 'requested' | 'scheduled' | 'completed' | 'cancelled' | 'no_show'
  price?: number
}

export interface Profile {
  name: string
  phone?: string
  notifications_on: boolean
  consultations: Consultation[]
}

export interface RequestBody {
  name: string
  phone: string
  email?: string
  category?: string
  question?: string
  slot?: string
}

export interface RequestResult {
  client_id: string
  consultation?: Consultation
}
