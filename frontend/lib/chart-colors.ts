// Validated data-viz colors (see the `dataviz` skill's reference palette —
// light mode only, this app has no dark theme). PRIMARY/SECONDARY are a
// categorical pair validated together (CVD ΔE 9.2 deutan, 27.6 normal-vision
// — passes both floors). STATUS is the fixed status palette: reserved for
// state, never reused as a series color, so a status color never impersonates
// a metric on this dashboard.
export const CHART_PRIMARY = '#1baf7a' // aqua/emerald — brand-adjacent, "the metric this page is about"
export const CHART_SECONDARY = '#eb6834' // orange — the comparison series

export const STATUS_GOOD = '#0ca30c'
export const STATUS_WARNING = '#fab219'
export const STATUS_SERIOUS = '#ec835a'
export const STATUS_CRITICAL = '#d03b3b'

export const GRIDLINE = '#e1e0d9'
export const AXIS_MUTED = '#898781'
