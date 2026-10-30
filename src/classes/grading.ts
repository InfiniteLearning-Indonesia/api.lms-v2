export interface GradingInput {
  rubrikAssessments: any[];
  competencyScores: { competencyId: string; score: number }[];
  assignmentScores: Map<string, number>;
  externalScores: Map<string, number>;
  isAttendanceCompetency?: (competencyId: string) => boolean;
}

export interface GradingAssessment {
  id: string;
  name: string;
  phase?: string;
  programCompetency?: { category?: string } | null;
  competencies?:
    | {
        competencyId: string;
        name?: string;
        competency?: { name: string };
        weight: number | string;
      }[]
    | null;
  subAssessments?:
    | {
        assessmentId: string;
        weight: number | string;
      }[]
    | null;
}

export interface GradingItem {
  id: string;
  name: string;
  category: 'Soft Skill' | 'Hard Skill';
  phase: 'Micro' | 'Massive';
  score: number;
}

export interface GradingResult {
  microItems: GradingItem[];
  massiveItems: GradingItem[];
  totalMicroScore: number;
  totalMassiveScore: number;
  finalScore: number;
  predicate: 'With Distinction' | 'Very Good' | 'Good' | 'Satisfactory';
  missingMassiveCount: number;
}

function ensureMin(value: number): number {
  if (!value || Number.isNaN(value) || value < 65) return 65;
  return Math.min(100, Math.round(value * 10) / 10);
}

export function computeGrading(input: GradingInput): GradingResult {
  const assessments = input.rubrikAssessments as GradingAssessment[];
  const byId = new Map(assessments.map((ra) => [ra.id, ra]));
  const direct = new Map(
    input.competencyScores.map((cs) => [cs.competencyId, cs.score]),
  );
  const cache = new Map<string, { score: number; hasScore: boolean }>();

  // Keep raw component values until the displayed item is clamped, as in my-grades.
  const calculate = (
    ra: GradingAssessment,
    visiting = new Set<string>(),
  ): { score: number; hasScore: boolean } => {
    if (input.externalScores.has(ra.id)) {
      return { score: Number(input.externalScores.get(ra.id)), hasScore: true };
    }
    if (visiting.has(ra.id)) return { score: 65, hasScore: false };
    const cached = cache.get(ra.id);
    if (cached) return cached;
    const path = new Set(visiting).add(ra.id);
    let score = 0;
    let hasScore = false;
    const components = ra.competencies ?? [];
    const children = ra.subAssessments ?? [];
    for (const item of components) {
      const name = (item.name ?? item.competency?.name ?? '').toLowerCase();
      const value = direct.has(item.competencyId)
        ? direct.get(item.competencyId)
        : input.assignmentScores.get(name);
      if (value !== undefined && value !== null) hasScore = true;
      score += Number(value ?? 65) * (parseFloat(String(item.weight)) || 0);
    }
    for (const item of children) {
      const target = byId.get(item.assessmentId);
      if (!target) continue;
      const child = calculate(target, path);
      score += child.score * (parseFloat(String(item.weight)) || 0);
      hasScore ||= child.hasScore;
    }
    const result = {
      score: components.length || children.length ? score : 65,
      hasScore,
    };
    cache.set(ra.id, result);
    return result;
  };

  const makeItem = (
    ra: GradingAssessment,
    phase: GradingItem['phase'],
  ): GradingItem => {
    const category = (ra.programCompetency?.category ?? '').toLowerCase();
    const name = ra.name.toLowerCase();
    const soft =
      category.includes('soft') ||
      [
        'cca',
        'communication',
        'collaboration',
        'adaptive',
        'project management',
        'leadership',
      ].some((keyword) => name.includes(keyword));
    return {
      id: ra.id,
      name: ra.name,
      category: soft ? 'Soft Skill' : 'Hard Skill',
      phase,
      score: ensureMin(calculate(ra).score),
    };
  };
  const microItems = assessments
    .filter((ra) => !ra.phase || ra.phase === 'Micro')
    .map((ra) => makeItem(ra, 'Micro'));
  const massive = assessments.filter((ra) => ra.phase === 'Massive');
  const massiveItems = massive.map((ra) => makeItem(ra, 'Massive'));
  const average = (items: GradingItem[]): number =>
    ensureMin(
      items.length
        ? items.reduce((total, item) => total + item.score, 0) / items.length
        : 65,
    );
  const totalMicroScore = average(microItems);
  const totalMassiveScore = average(massiveItems);
  const finalScore = ensureMin(totalMicroScore * 0.4 + totalMassiveScore * 0.6);
  const predicate =
    finalScore >= 90
      ? 'With Distinction'
      : finalScore >= 80
        ? 'Very Good'
        : finalScore >= 70
          ? 'Good'
          : 'Satisfactory';
  return {
    microItems,
    massiveItems,
    totalMicroScore,
    totalMassiveScore,
    finalScore,
    predicate,
    missingMassiveCount: massive.filter((ra) => !calculate(ra).hasScore).length,
  };
}
