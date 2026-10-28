export interface MentorProgramScope {
  mode: 'all' | 'programs' | 'own';
  patterns: string[];
}

export function resolveMentorProgramScope(
  specialization?: string | null,
): MentorProgramScope {
  const spec = (specialization ?? '').toLowerCase();

  if (spec.includes('prof')) return { mode: 'all', patterns: [] };
  if (spec.includes('ui') || spec.includes('ux')) {
    return {
      mode: 'programs',
      patterns: ['web development', 'mobile development'],
    };
  }
  if (spec.includes('web')) {
    return { mode: 'programs', patterns: ['web development'] };
  }
  if (spec.includes('game')) {
    return { mode: 'programs', patterns: ['game development'] };
  }
  if (spec.includes('ai') || spec.includes('artificial')) {
    return { mode: 'programs', patterns: ['ai development'] };
  }
  return { mode: 'own', patterns: [] };
}
