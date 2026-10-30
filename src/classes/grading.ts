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
  hasScore: boolean;
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

export const MISSING_SCORE_FLOOR = 75;

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

  // Keep raw component values until the displayed item is rounded and clamped.
  const calculate = (
    ra: GradingAssessment,
    visiting = new Set<string>(),
  ): { score: number; hasScore: boolean } => {
    if (input.externalScores.has(ra.id)) {
      return { score: Number(input.externalScores.get(ra.id)), hasScore: true };
    }
    if (visiting.has(ra.id)) {
      return { score: MISSING_SCORE_FLOOR, hasScore: false };
    }
    const cached = cache.get(ra.id);
    if (cached) return cached;
    const path = new Set(visiting).add(ra.id);
    let score = 0;
    let hasScore = false;
    const components = ra.competencies ?? [];
    const children = ra.subAssessments ?? [];
    for (const item of components) {
      const name = (item.name ?? item.competency?.name ?? '').toLowerCase();
      const value =
        direct.get(item.competencyId) ?? input.assignmentScores.get(name);
      if (value === undefined || value === null) continue;
      hasScore = true;
      score += Number(value) * (parseFloat(String(item.weight)) || 0);
    }
    // Subassessments are a fallback when no competency component has a score.
    if (!hasScore) {
      for (const item of children) {
        const target = byId.get(item.assessmentId);
        if (!target) continue;
        const child = calculate(target, path);
        if (!child.hasScore) continue;
        score += child.score * (parseFloat(String(item.weight)) || 0);
        hasScore = true;
      }
    }
    const result = {
      score: hasScore ? score : MISSING_SCORE_FLOOR,
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
    const { score, hasScore } = calculate(ra);
    return {
      id: ra.id,
      name: ra.name,
      category: soft ? 'Soft Skill' : 'Hard Skill',
      phase,
      score: hasScore ? ensureMin(score) : MISSING_SCORE_FLOOR,
      hasScore,
    };
  };
  const microItems = assessments
    .filter((ra) => !ra.phase || ra.phase === 'Micro')
    .map((ra) => makeItem(ra, 'Micro'));
  const massiveItems = assessments
    .filter((ra) => ra.phase === 'Massive')
    .map((ra) => makeItem(ra, 'Massive'));
  const average = (items: GradingItem[]): number =>
    ensureMin(
      items.length
        ? items.reduce((total, item) => total + item.score, 0) / items.length
        : MISSING_SCORE_FLOOR,
    );
  const totalMicroScore = average(microItems);
  const totalMassiveScore = average(massiveItems);
  const finalScore = average([...microItems, ...massiveItems]);
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
    missingMassiveCount: massiveItems.filter((item) => !item.hasScore).length,
  };
}
