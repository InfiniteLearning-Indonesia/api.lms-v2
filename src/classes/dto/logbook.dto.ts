import { IsString, IsNotEmpty, IsNumber, Min } from 'class-validator';

export class CreateLogbookDto {
  @IsNumber()
  @Min(1)
  monthIndex: number;

  @IsString()
  @IsNotEmpty()
  q1_experience: string;

  @IsString()
  @IsNotEmpty()
  q2_progress: string;

  @IsString()
  @IsNotEmpty()
  q3_challenges: string;

  @IsString()
  @IsNotEmpty()
  q4_competencies: string;
}
