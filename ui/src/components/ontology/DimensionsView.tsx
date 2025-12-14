import { LayoutGrid } from "lucide-react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../ui/Card";

const DimensionsView = () => {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <LayoutGrid className="h-5 w-5 text-green-500" />
          <CardTitle>Dimensions (Grouping Attributes)</CardTitle>
        </div>
        <CardDescription>
          Attributes for grouping and aggregating data in analytics queries
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div className="text-sm text-text-tertiary italic">
            Dimensions are not yet available in the tiered ontology system.
            This feature will be added in a future update.
          </div>
        </div>
      </CardContent>
    </Card>
  );
};

export default DimensionsView;
