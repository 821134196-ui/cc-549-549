// 金额：后端一律以“分”为整数单位
export function yuan(fen) {
  if (fen === null || fen === undefined) return '—'
  const neg = fen < 0
  const v = Math.abs(fen)
  const s = (v / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return (neg ? '−¥' : '¥') + s
}

export function pct(bps) {
  if (bps === null || bps === undefined) return '—'
  return (bps / 100).toFixed(2).replace(/\.?0+$/, '') + '%'
}

export const EXCEPTION_LABELS = {
  cross_region: { label: '越区销售', cls: 'ex' },
  cross_period_return: { label: '跨期退货', cls: 'ex' },
  post_expiry: { label: '合同到期后补传', cls: 'ex' },
  language_not_licensed: { label: '授权语种之外', cls: 'ex' },
  no_contract: { label: '销售日无有效合同', cls: 'ex' },
  settlement_locked: { label: '结算单已确认·须补差', cls: 'ex' },
}

export const STATUS_LABELS = {
  draft: { label: '草稿', cls: 'st-draft' },
  confirmed: { label: '已确认·冻结', cls: 'st-confirmed' },
  paid: { label: '已付款', cls: 'st-paid' },
}

export const LINE_STATUS = {
  normal: { label: '正常', cls: 'ok' },
  settled: { label: '已计酬', cls: 'ok' },
  exception: { label: '异常隔离', cls: 'ex' },
}

export function fmtDate(s) {
  return s || '—'
}
