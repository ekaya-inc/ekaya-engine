import { Filter } from "lucide-react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "../ui/Card";

const FiltersView = () => {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Filter className="h-5 w-5 text-blue-500" />
          <CardTitle>Predefined Filters</CardTitle>
        </div>
        <CardDescription>
          Common WHERE conditions that users can apply to queries
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div className="text-sm text-text-tertiary italic">
            Filters are not yet available in the tiered ontology system.
            This feature will be added in a future update.
          </div>
        </div>
      </CardContent>
    </Card>
  );
};

export default FiltersView;
