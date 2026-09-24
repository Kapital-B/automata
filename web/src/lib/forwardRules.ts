import type {
  CategoryDefinition,
  ForwardCondition,
  ForwardField,
  ForwardOp,
  ForwardPredicate,
  ForwardRule,
} from "@/lib/auth";

export const fieldLabels: Record<ForwardField, string> = {
  category_slug: "Category",
  has_attachments: "Attachments",
  from: "Sender",
  subject: "Subject",
};

/** The operators each field allows, mirroring the server's validation. */
export const fieldOps: Record<ForwardField, ForwardOp[]> = {
  category_slug: ["equals"],
  has_attachments: ["equals"],
  from: ["domain", "equals", "contains"],
  subject: ["contains", "equals"],
};

export const opLabels: Record<ForwardField, Partial<Record<ForwardOp, string>>> = {
  category_slug: { equals: "is" },
  has_attachments: { equals: "" },
  from: { domain: "is from domain", equals: "is exactly", contains: "contains" },
  subject: { contains: "contains", equals: "is exactly" },
};

export function defaultPredicate(field: ForwardField, categories: CategoryDefinition[]): ForwardPredicate {
  switch (field) {
    case "has_attachments":
      return { field, op: "equals", value: true };
    case "category_slug":
      return { field, op: "equals", value: categories[0]?.slug ?? "finance" };
    default:
      return { field, op: fieldOps[field][0], value: "" };
  }
}

export function isLogic(condition: ForwardCondition): condition is { all: ForwardPredicate[] } {
  return "all" in condition && Array.isArray(condition.all);
}

function categoryName(slug: string, categories: CategoryDefinition[]) {
  return categories.find((c) => c.slug === slug)?.display_name ?? slug;
}

export function describePredicate(p: ForwardPredicate, categories: CategoryDefinition[]): string {
  switch (p.field) {
    case "has_attachments":
      return p.value ? "has attachments" : "has no attachments";
    case "category_slug":
      return `category is ${categoryName(String(p.value), categories)}`;
    case "from":
      return p.op === "domain"
        ? `sender is from ${p.value}`
        : p.op === "equals"
          ? `sender is ${p.value}`
          : `sender contains “${p.value}”`;
    case "subject":
      return p.op === "equals" ? `subject is “${p.value}”` : `subject contains “${p.value}”`;
    default:
      return "";
  }
}

/** A rule's condition as a sentence, so nobody has to read JSON. */
export function describeCondition(condition: ForwardCondition, categories: CategoryDefinition[]): string {
  if (isLogic(condition)) {
    const parts = condition.all.map((p) => describePredicate(p, categories)).filter(Boolean);
    if (parts.length === 0) return "No conditions";
    const text = parts.join(" and ");
    return text.charAt(0).toUpperCase() + text.slice(1);
  }
  return `AI decides: “${condition.prompt}”`;
}

/** Whether a predicate has what it needs to be saved. */
export function predicateComplete(p: ForwardPredicate): boolean {
  return typeof p.value === "boolean" || String(p.value).trim() !== "";
}

// Deliberately loose: the server has the final word on addresses.
export function looksLikeEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}

/** The epoch start means the rule was switched on for existing mail too. */
export function coversExistingMail(rule: ForwardRule): boolean {
  return new Date(rule.applies_from).getTime() <= 0;
}

export function ruleState(rule: ForwardRule): "on" | "paused" | "blocked" {
  if (!rule.enabled) return "paused";
  return rule.blocked_reason ? "blocked" : "on";
}
