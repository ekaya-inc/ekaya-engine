import { ArrowLeft, Database } from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import QueriesView, { type Query } from "../components/QueriesView";
import { Button } from "../components/ui/Button";

const QueriesPage = () => {
  const navigate = useNavigate();
  const { pid } = useParams<{ pid: string }>();
  // Queries state - moved from OntologyPage
  const [queries, setQueries] = useState<Query[]>([
    {
      id: 1,
      naturalLanguagePrompt:
        "Show me the top 10 customers by revenue this month",
      additionalContext:
        "Consider only completed orders and exclude refunded transactions",
      sqlQuery: `SELECT
  c.customer_id,
  c.name,
  SUM(o.total_amount) as total_revenue
FROM customers c
JOIN orders o ON c.customer_id = o.customer_id
WHERE o.status = 'completed'
  AND o.order_date >= DATE_TRUNC('month', CURRENT_DATE)
  AND o.refunded = false
GROUP BY c.customer_id, c.name
ORDER BY total_revenue DESC
LIMIT 10;`,
      category: "analytics",
      isActive: true,
      createdAt: new Date("2024-01-15"),
      lastUsed: new Date("2024-01-20"),
      usageCount: 15,
    },
    {
      id: 2,
      naturalLanguagePrompt: "What are our daily sales for the last 7 days?",
      additionalContext: "Include both online and in-store sales",
      sqlQuery: `SELECT
  DATE(o.order_date) as sale_date,
  COUNT(*) as order_count,
  SUM(o.total_amount) as daily_revenue
FROM orders o
WHERE o.order_date >= CURRENT_DATE - INTERVAL '7 days'
  AND o.status = 'completed'
GROUP BY DATE(o.order_date)
ORDER BY sale_date DESC;`,
      category: "reporting",
      isActive: true,
      createdAt: new Date("2024-01-10"),
      lastUsed: new Date("2024-01-22"),
      usageCount: 8,
    },
    {
      id: 3,
      naturalLanguagePrompt: "Which products are running low on inventory?",
      additionalContext: "Alert when stock is below 10 units",
      sqlQuery: `SELECT
  p.product_id,
  p.name,
  p.current_stock,
  p.reorder_level
FROM products p
WHERE p.current_stock < 10
ORDER BY p.current_stock ASC;`,
      category: "operational",
      isActive: true,
      createdAt: new Date("2024-01-12"),
      lastUsed: null,
      usageCount: 0,
    },
    {
      id: 4,
      naturalLanguagePrompt: "Show me all orders that need immediate attention",
      additionalContext:
        "Orders that are overdue for shipping or have payment issues",
      sqlQuery: `SELECT
  o.order_id,
  o.customer_id,
  o.order_date,
  o.status,
  o.total_amount
FROM orders o
WHERE (o.status = 'payment_pending' AND o.order_date < CURRENT_DATE - INTERVAL '2 days')
   OR (o.status = 'processing' AND o.order_date < CURRENT_DATE - INTERVAL '3 days')
ORDER BY o.order_date ASC;`,
      category: "critical",
      isActive: true,
      createdAt: new Date("2024-01-08"),
      lastUsed: new Date("2024-01-21"),
      usageCount: 23,
    },
  ]);

  return (
    <div className="mx-auto max-w-7xl">
      {/* Header */}
      <div className="mb-6">
        <Button
          variant="ghost"
          onClick={() => navigate(`/projects/${pid}`)}
          className="mb-4"
        >
          <ArrowLeft className="mr-2 h-4 w-4" />
          Back to Dashboard
        </Button>
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-text-primary flex items-center gap-2">
              <Database className="h-8 w-8 text-blue-500" />
              Query Management
            </h1>
            <p className="mt-2 text-text-secondary">
              Manage pre-approved natural language queries and their
              corresponding SQL
            </p>
          </div>
        </div>
      </div>

      {/* Queries Management Interface */}
      <QueriesView queries={queries} setQueries={setQueries} />
    </div>
  );
};

export default QueriesPage;
