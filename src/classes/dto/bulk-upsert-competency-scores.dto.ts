import { Type } from 'class-transformer';
import {
  ArrayMinSize,
  IsArray,
  IsNumber,
  IsUUID,
  Max,
  Min,
  ValidateNested,
} from 'class-validator';

export class CompetencyScoreItemDto {
  @IsUUID()
  studentId: string;

  @IsUUID()
  competencyId: string;

  @IsNumber()
  @Min(0)
  @Max(100)
  score: number;
}

export class BulkUpsertCompetencyScoresDto {
  @IsArray()
  @ArrayMinSize(1)
  @ValidateNested({ each: true })
  @Type(() => CompetencyScoreItemDto)
  scores: CompetencyScoreItemDto[];
}
