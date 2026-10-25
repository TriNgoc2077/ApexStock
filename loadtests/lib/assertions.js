import { Rate, Trend, Counter } from 'k6/metrics';

export const customMetrics = {
  successOrders: new Counter('orders_success_total'),
  businessRejections: new Counter('orders_business_rejection_total'),
  systemFailures: new Counter('orders_system_failure_total'),

  successRate: new Rate('rate_orders_success'),
  businessRejectionRate: new Rate('rate_orders_business_rejection'),
  systemFailureRate: new Rate('rate_orders_system_failure'),

  orderLatency: new Trend('order_request_duration_ms', true),
};

export function classifyResponse(res) {
  customMetrics.orderLatency.add(res.timings.duration);

  const status = res.status;

  if (status === 201) {
    customMetrics.successOrders.add(1);
    customMetrics.successRate.add(true);
    customMetrics.businessRejectionRate.add(false);
    customMetrics.systemFailureRate.add(false);
    return 'SUCCESS';
  }

  // 409 Conflict (sold out, max per user) or 429 Rate limited are valid business rejections
  if (status === 409 || status === 429) {
    customMetrics.businessRejections.add(1);
    customMetrics.successRate.add(false);
    customMetrics.businessRejectionRate.add(true);
    customMetrics.systemFailureRate.add(false);
    return 'BUSINESS_REJECTION';
  }

  // 5xx errors or connection timeouts are system failures
  customMetrics.systemFailures.add(1);
  customMetrics.successRate.add(false);
  customMetrics.businessRejectionRate.add(false);
  customMetrics.systemFailureRate.add(true);
  return 'SYSTEM_FAILURE';
}
