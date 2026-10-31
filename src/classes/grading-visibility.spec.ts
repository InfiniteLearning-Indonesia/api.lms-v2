import { computeGrading, GradingAssessment } from './grading';
import { isHiddenItem } from './grading-visibility';

const assessments: GradingAssessment[] = [
  { id: 'ui-micro', name: 'UI/UX Design', phase: 'Micro' },
  { id: 'cca', name: 'CCA', phase: 'Micro' },
  { id: 'career', name: 'Career Coaching', phase: 'Micro' },
  { id: 'ml', name: 'Machine Learning', phase: 'Micro' },
  { id: 'ui-massive', name: 'UI Design Project', phase: 'Massive' },
  { id: 'ux-massive', name: 'UX Design Project', phase: 'Massive' },
  { id: 'ml-massive', name: 'Machine Learning Project', phase: 'Massive' },
];

function grade(programName?: string | null) {
  return computeGrading({
    programName,
    rubrikAssessments: assessments,
    competencyScores: [],
    assignmentScores: new Map(),
    externalScores: new Map([
      ['ui-micro', 100],
      ['cca', 80],
      ['career', 85],
      ['ml', 90],
      ['ui-massive', 95],
      ['ml-massive', 85],
    ]),
  });
}

describe('grading visibility', () => {
  it.each(['AI Development', 'ai development', 'AI DEVELOPMENT'])(
    'hides UI/UX rows in both phases for %s and preserves other items',
    (programName) => {
      const result = grade(programName);
      const unfiltered = grade();
      expect(result.microItems.map((item) => item.name)).toEqual([
        'CCA',
        'Career Coaching',
        'Machine Learning',
      ]);
      expect(result.massiveItems.map((item) => item.name)).toEqual([
        'Machine Learning Project',
      ]);
      expect(result.microItems).toEqual(unfiltered.microItems.slice(1));
      expect(result.massiveItems).toEqual(unfiltered.massiveItems.slice(2));
      expect(result).toMatchObject({
        totalMicroScore: unfiltered.totalMicroScore,
        totalMassiveScore: unfiltered.totalMassiveScore,
        finalScore: unfiltered.finalScore,
        predicate: unfiltered.predicate,
        missingMassiveCount: unfiltered.missingMassiveCount,
      });
    },
  );

  it.each([
    'UI/UX Design',
    'ui ux design',
    'UIUX Design',
    'UI Design',
    'ux design',
  ])('matches configured item pattern: %s', (name) =>
    expect(isHiddenItem('AI Development', name)).toBe(true),
  );

  it('preserves design items for Web Development and UI/UX Design', () => {
    const names = [
      'User Interface Design',
      'User Experience Design',
      'UI/UX Design',
    ];
    const result = computeGrading({
      programName: 'Web Development and UI/UX Design',
      rubrikAssessments: names.map((name) => ({
        id: name,
        name,
        phase: 'Micro',
      })),
      competencyScores: [],
      assignmentScores: new Map(),
      externalScores: new Map(),
    });
    expect(result.microItems.map((item) => item.name)).toEqual(names);
  });

  it.each([null, undefined, ''])(
    'preserves every item when program name is %s',
    (programName) => {
      expect(grade(programName)).toEqual(grade('Unconfigured Program'));
      expect(isHiddenItem(programName, 'UI/UX Design')).toBe(false);
    },
  );
});
