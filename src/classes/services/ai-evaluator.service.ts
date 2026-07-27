import { Injectable, Logger, BadRequestException } from '@nestjs/common';

export interface ModelOption {
  id: string;
  name: string;
}

export interface EvaluationResult {
  score: number;
  feedback: string;
  analysis?: string;
  prompt?: string;
  isVideo?: boolean;
}

@Injectable()
export class AiEvaluatorService {
  private readonly logger = new Logger(AiEvaluatorService.name);

  // 1. Dynamic Models Fetcher
  async fetchModels(provider: string, hostOrApiKey?: string): Promise<ModelOption[]> {
    try {
      if (provider === 'ollama') {
        const host = (hostOrApiKey || 'http://localhost:11434').replace(/\/$/, '');
        const res = await fetch(`${host}/api/tags`);
        if (!res.ok) throw new Error(`Ollama returned status ${res.status}`);
        const data = await res.json();
        const models = (data.models || []).map((m: any) => ({
          id: m.name || m.model,
          name: m.name || m.model,
        }));
        return models.length > 0 ? models : [{ id: 'llama3', name: 'llama3 (Default)' }];
      }

      if (provider === 'groq') {
        if (!hostOrApiKey) throw new BadRequestException('Groq API Key is required');
        const res = await fetch('https://api.groq.com/openai/v1/models', {
          headers: { Authorization: `Bearer ${hostOrApiKey}` },
        });
        if (!res.ok) throw new Error(`Groq returned status ${res.status}`);
        const data = await res.json();
        const models = (data.data || []).map((m: any) => ({
          id: m.id,
          name: m.id,
        }));
        return models.length > 0
          ? models
          : [{ id: 'llama-3.3-70b-versatile', name: 'llama-3.3-70b-versatile' }];
      }

      if (provider === 'gemini') {
        if (!hostOrApiKey) throw new BadRequestException('Google AI Studio Key is required');
        const res = await fetch(
          `https://generativelanguage.googleapis.com/v1beta/models?key=${hostOrApiKey}`,
        );
        if (!res.ok) throw new Error(`Gemini API returned status ${res.status}`);
        const data = await res.json();
        const models = (data.models || [])
          .filter((m: any) => m.supportedGenerationMethods?.includes('generateContent'))
          .map((m: any) => ({
            id: m.name.replace(/^models\//, ''),
            name: m.displayName || m.name.replace(/^models\//, ''),
          }));
        return models.length > 0
          ? models
          : [{ id: 'gemini-1.5-flash', name: 'gemini-1.5-flash' }];
      }

      throw new BadRequestException(`Provider '${provider}' is not supported.`);
    } catch (err: any) {
      this.logger.error(`Error fetching models for ${provider}: ${err.message}`);
      if (err instanceof BadRequestException) {
        throw err;
      }
      return [];
    }
  }

  // 2. Pre-detect Video Links
  detectSubmissionMedia(link: string): { isVideo: boolean; reason?: string } {
    if (!link) return { isVideo: false };

    const lower = link.toLowerCase();
    const videoExtensions = ['.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.m4v'];
    if (videoExtensions.some((ext) => lower.includes(ext))) {
      return { isVideo: true, reason: 'File format video (.mp4, .mov, dll) tidak didukung AI.' };
    }

    if (lower.includes('youtube.com') || lower.includes('youtu.be') || lower.includes('vimeo.com')) {
      return { isVideo: true, reason: 'Link platform video (YouTube / Vimeo) tidak dapat dievaluasi otomatis oleh AI.' };
    }

    return { isVideo: false };
  }

  // 3. Link Inspection Engine
  async inspectLinkContent(
    link: string,
    tokens?: { githubToken?: string | null; figmaToken?: string | null },
  ): Promise<string> {
    if (!link) return 'Link pengumpulan kosong.';

    try {
      const lower = link.toLowerCase();

      // A. GitHub Repository
      if (lower.includes('github.com')) {
        return await this.inspectGitHubRepo(link, tokens?.githubToken);
      }

      // B. Figma Design
      if (lower.includes('figma.com')) {
        return await this.inspectFigmaDesign(link, tokens?.figmaToken);
      }

      // C. Google Docs / Sheets
      if (lower.includes('docs.google.com') || lower.includes('drive.google.com')) {
        return await this.inspectGoogleDocOrDrive(link);
      }

      // D. Direct Public Text/Code Link
      const res = await fetch(link, { signal: AbortSignal.timeout(5000) });
      if (res.ok) {
        const text = await res.text();
        return text.substring(0, 3000);
      }
    } catch (err: any) {
      this.logger.warn(`Could not deeply inspect link ${link}: ${err.message}`);
    }

    return `Tautan pengumpulan: ${link}`;
  }

  private async inspectGitHubRepo(url: string, githubToken?: string | null): Promise<string> {
    try {
      const cleanToken = githubToken ? githubToken.trim() : null;
      const headers: Record<string, string> = {
        Accept: 'application/vnd.github.v3+json',
        'User-Agent': 'LMS-AI-Evaluator',
      };
      if (cleanToken) {
        headers['Authorization'] = cleanToken.startsWith('github_pat_') ? `Bearer ${cleanToken}` : `token ${cleanToken}`;
      }

      // Check if direct file link (e.g. github.com/owner/repo/blob/main/app.py)
      const blobMatch = url.match(/github\.com\/([^\/]+)\/([^\/]+)\/blob\/([^\/]+)\/(.+)/);
      if (blobMatch) {
        const [, owner, repo, branch, filePath] = blobMatch;
        const rawUrl = `https://raw.githubusercontent.com/${owner}/${repo}/${branch}/${filePath}`;
        const rawRes = await fetch(rawUrl);
        if (rawRes.ok) {
          const codeText = await rawRes.text();
          return `GitHub File Spesifik: ${owner}/${repo}/${filePath}
Isi Kode Sumber:
${codeText.substring(0, 6000)}`;
        }
      }

      const match = url.match(/github\.com\/([^\/]+)\/([^\/]+)/);
      if (!match) return `GitHub link: ${url}`;
      const [, owner, repoRaw] = match;
      const repo = repoRaw.replace(/\.git$/, '');

      // Fetch repo tree
      let contentsRes = await fetch(`https://api.github.com/repos/${owner}/${repo}/contents`, { headers });

      // Retry without token if authenticated call returned 401/403 (for public repo resiliency)
      if (!contentsRes.ok && cleanToken && (contentsRes.status === 401 || contentsRes.status === 403)) {
        const publicHeaders = {
          Accept: 'application/vnd.github.v3+json',
          'User-Agent': 'LMS-AI-Evaluator',
        };
        contentsRes = await fetch(`https://api.github.com/repos/${owner}/${repo}/contents`, { headers: publicHeaders });
      }

      if (!contentsRes.ok) {
        return `GitHub Repository ${owner}/${repo} (Privat atau tidak dapat diakses). URL: ${url}`;
      }

      const files = await contentsRes.json();
      if (!Array.isArray(files)) return `GitHub Repo: ${owner}/${repo}`;

      const fileNames = files.map((f: any) => `${f.name}${f.type === 'dir' ? '/' : ''}`).join(', ');

      // Try fetching README
      let readmeText = '';
      const readmeFile = files.find((f: any) => f.name.toLowerCase() === 'readme.md');
      if (readmeFile && readmeFile.download_url) {
        try {
          const rRes = await fetch(readmeFile.download_url);
          if (rRes.ok) readmeText = await rRes.text();
        } catch (e) {}
      }

      // Collect code files to inspect
      const codeExtensions = ['.py', '.js', '.ts', '.jsx', '.tsx', '.html', '.css', '.json', '.sql', '.java', '.cpp', '.c', '.php', '.ipynb', '.txt'];
      let candidateFiles: any[] = files.filter((f: any) => {
        if (f.type !== 'file') return false;
        const name = f.name.toLowerCase();
        if (name === 'readme.md' || name.endsWith('.lock') || name.startsWith('.')) return false;
        return codeExtensions.some((ext) => name.endsWith(ext));
      });

      // If top level has subdirectories like src/ or app/, inspect subdirectories for code files
      const dirFiles = files.filter((f: any) => f.type === 'dir' && !f.name.startsWith('.'));
      for (const dir of dirFiles.slice(0, 2)) {
        try {
          let subRes = await fetch(`https://api.github.com/repos/${owner}/${repo}/contents/${dir.path}`, { headers });
          if (!subRes.ok && cleanToken) {
            subRes = await fetch(`https://api.github.com/repos/${owner}/${repo}/contents/${dir.path}`);
          }
          if (subRes.ok) {
            const subItems = await subRes.json();
            if (Array.isArray(subItems)) {
              const subCodeFiles = subItems.filter((f: any) => {
                if (f.type !== 'file') return false;
                const name = f.name.toLowerCase();
                return codeExtensions.some((ext) => name.endsWith(ext));
              });
              candidateFiles = [...candidateFiles, ...subCodeFiles];
            }
          }
        } catch (e) {}
      }

      // Read content for top 6 code files
      let sourceCodeSnippets = '';
      const targetCodeFiles = candidateFiles.slice(0, 6);
      for (const file of targetCodeFiles) {
        if (file.download_url) {
          try {
            const fRes = await fetch(file.download_url);
            if (fRes.ok) {
              const code = await fRes.text();
              sourceCodeSnippets += `\n--- File: ${file.path || file.name} ---\n${code.substring(0, 1500)}\n`;
            }
          } catch (err) {}
        }
      }

      return `GitHub Repository: ${owner}/${repo}
Daftar File / Folder: ${fileNames}
${readmeText ? `\n[README Content]:\n${readmeText.substring(0, 1500)}\n` : ''}
[Cuplikan Kode Sumber (Source Code Files)]:
${sourceCodeSnippets || '(Tidak ada file kode teks utama yang dapat dibaca)'}`;
    } catch (e: any) {
      return `GitHub Link: ${url}`;
    }
  }

  private async inspectFigmaDesign(url: string, figmaToken?: string | null): Promise<string> {
    try {
      const match = url.match(/figma\.com\/(file|design)\/([a-zA-Z0-9]+)/);
      if (!match) return `Figma Link: ${url}`;
      const fileKey = match[2];

      if (!figmaToken) {
        return `Figma Design (File Key: ${fileKey}). Mentee mengumpulkan desain Figma. (Token Figma belum diset mentor). URL: ${url}`;
      }

      const res = await fetch(`https://api.figma.com/v1/files/${fileKey}`, {
        headers: { 'X-Figma-Token': figmaToken },
      });

      if (!res.ok) {
        return `Figma Design (File Key: ${fileKey}). Tidak dapat membaca detail file Figma. URL: ${url}`;
      }

      const data = await res.json();
      const document = data.document;
      const pages = document?.children || [];
      const pageNames = pages.map((p: any) => p.name).join(', ');
      let frameCount = 0;
      pages.forEach((p: any) => {
        if (p.children) frameCount += p.children.length;
      });

      return `Figma File: ${data.name || fileKey}
Dibuat: ${data.lastModified}
Halaman Desain: ${pageNames}
Total Frame/Canvas: ${frameCount}
Link Desain: ${url}`;
    } catch (e: any) {
      return `Figma Link: ${url}`;
    }
  }

  private async inspectGoogleDocOrDrive(url: string): Promise<string> {
    try {
      if (url.includes('docs.google.com/document/d/')) {
        const match = url.match(/document\/d\/([a-zA-Z0-9_-]+)/);
        if (match) {
          const docId = match[1];
          const exportUrl = `https://docs.google.com/document/d/${docId}/export?format=txt`;
          const res = await fetch(exportUrl);
          if (res.ok) {
            const text = await res.text();
            return `Konten Google Docs:
${text.substring(0, 3000)}`;
          }
        }
      }

      if (url.includes('docs.google.com/spreadsheets/d/')) {
        const match = url.match(/spreadsheets\/d\/([a-zA-Z0-9_-]+)/);
        if (match) {
          const sheetId = match[1];
          const exportUrl = `https://docs.google.com/spreadsheets/d/${sheetId}/export?format=csv`;
          const res = await fetch(exportUrl);
          if (res.ok) {
            const csv = await res.text();
            return `Konten Google Sheets (CSV Preview):
${csv.substring(0, 3000)}`;
          }
        }
      }
    } catch (e: any) {
      // Fallback
    }
    return `Tautan Google Drive/Docs: ${url}`;
  }

  // 4. Multi-Provider AI LLM Execution Engine
  async evaluateSubmissionWithAi(params: {
    link: string;
    competencyName?: string;
    rubric?: any;
    assignmentTitle: string;
    assignmentInstruction?: string;
    provider: string;
    hostOrApiKey?: string;
    model?: string;
    githubToken?: string | null;
    figmaToken?: string | null;
  }): Promise<EvaluationResult> {
    const {
      link,
      competencyName,
      rubric,
      assignmentTitle,
      assignmentInstruction,
      provider,
      hostOrApiKey,
      model,
      githubToken,
      figmaToken,
    } = params;

    // A. Pre-detect Video
    const mediaCheck = this.detectSubmissionMedia(link);
    if (mediaCheck.isVideo) {
      return {
        score: 0,
        feedback: mediaCheck.reason || 'File video tidak didukung untuk evaluasi otomatis AI.',
        analysis: 'Penilaian dilewati karena format berupa video.',
        isVideo: true,
      };
    }

    // B. Inspect link contents
    const inspectedContent = await this.inspectLinkContent(link, { githubToken, figmaToken });

    // C. Construct Rubric & Prompt
    const prompt = `Anda adalah penilai tugas akademis otomatis yang profesional & objektif di LMS Infinite Learning.
Tugas: ${assignmentTitle}
Instruksi Tugas: ${assignmentInstruction || 'Kerjakan tugas sesuai instruksi.'}
Kompetensi Terkait: ${competencyName || 'General'}
Rubrik Penilaian Terpilih: ${JSON.stringify(rubric || { standard: 'Lengkap, sesuai instruksi, dan tanpa kesalahan fatal.' })}

PERHATIAN KHUSUS EVALUASI:
- Evaluasi HANYA berdasarkan Kriteria Rubrik Terpilih di atas dan Instruksi Tugas.
- JANGAN menilai atau mengurangi poin berdasarkan kriteria/materi di luar rubrik spesifik ini.

Tautan Pengumpulan Mentee: ${link}
Hasil Inspeksi Isi Tautan:
${inspectedContent}

Tugas Anda:
1. Berikan nilai (score) berupa angka bulat dari 0 hingga 100 berdasarkan kesesuaian dengan rubrik spesifik ini.
2. Berikan umpan balik (feedback) yang konstruktif dan sopan dalam bahasa Indonesia.
3. Kembalikan HANYA format JSON valid tanpa tanda backtick markdown:
{"score": 85, "feedback": "Penjelasan umpan balik..."}`;

    try {
      let rawResponse = '';

      if (provider === 'ollama') {
        const host = (hostOrApiKey || 'http://localhost:11434').replace(/\/$/, '');
        const targetModel = model || 'llama3';
        try {
          const res = await fetch(`${host}/api/generate`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              model: targetModel,
              prompt,
              stream: false,
              format: 'json',
            }),
          });

          if (!res.ok) {
            const errText = await res.text().catch(() => '');
            if (res.status === 429 || errText.toLowerCase().includes('rate limit') || errText.toLowerCase().includes('memory')) {
              const rateErr: any = new Error(`⚠️ Server Ollama lokal kehabisan memori / terbatasi (${res.statusText}).`);
              rateErr.isRateLimit = true;
              throw rateErr;
            }
            throw new Error(`Server Ollama merespons error (${res.status}): ${errText || res.statusText}`);
          }
          const data = await res.json();
          rawResponse = data.response;
        } catch (fetchErr: any) {
          if (fetchErr.isRateLimit) throw fetchErr;
          const offlineErr: any = new Error(`⚠️ Server Ollama tidak aktif di ${host}. Pastikan Ollama sudah berjalan.`);
          offlineErr.isOffline = true;
          throw offlineErr;
        }
      } else if (provider === 'groq') {
        if (!hostOrApiKey) throw new BadRequestException('Groq API Key tidak ditemukan.');
        const targetModel = model || 'llama-3.3-70b-versatile';
        const res = await fetch('https://api.groq.com/openai/v1/chat/completions', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${hostOrApiKey}`,
          },
          body: JSON.stringify({
            model: targetModel,
            messages: [
              { role: 'system', content: 'You are an LMS AI Grader. Return pure JSON object with score and feedback.' },
              { role: 'user', content: prompt },
            ],
            response_format: { type: 'json_object' },
          }),
        });

        if (!res.ok) {
          const errBody = await res.text().catch(() => '');
          if (res.status === 429 || errBody.toLowerCase().includes('rate_limit') || errBody.toLowerCase().includes('quota')) {
            const rateErr: any = new Error('⚠️ Batas Rate Limit / Kuota Provider Groq Terlampaui (HTTP 429). Silakan tunggu beberapa saat atau ganti Provider AI.');
            rateErr.isRateLimit = true;
            throw rateErr;
          }
          throw new Error(`Groq API Error (${res.status}): ${errBody || res.statusText}`);
        }
        const data = await res.json();
        rawResponse = data.choices?.[0]?.message?.content || '';
      } else if (provider === 'gemini') {
        if (!hostOrApiKey) throw new BadRequestException('Google AI Studio Key tidak ditemukan.');
        const targetModel = model || 'gemini-1.5-flash';
        const res = await fetch(
          `https://generativelanguage.googleapis.com/v1beta/models/${targetModel}:generateContent?key=${hostOrApiKey}`,
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              contents: [{ parts: [{ text: prompt }] }],
              generationConfig: { responseMimeType: 'application/json' },
            }),
          },
        );

        if (!res.ok) {
          const errBody = await res.text().catch(() => '');
          if (res.status === 429 || errBody.includes('RESOURCE_EXHAUSTED') || errBody.toLowerCase().includes('quota')) {
            const rateErr: any = new Error('⚠️ Batas Kuota Google AI Studio (Gemini) Terlampaui (HTTP 429 / RESOURCE_EXHAUSTED). Silakan ganti API Key atau tunggu reset kuota harian.');
            rateErr.isRateLimit = true;
            throw rateErr;
          }
          throw new Error(`Google AI Studio Error (${res.status}): ${errBody || res.statusText}`);
        }
        const data = await res.json();
        rawResponse = data.candidates?.[0]?.content?.parts?.[0]?.text || '';
      } else {
        throw new BadRequestException(`Provider '${provider}' tidak dikenal.`);
      }

      // Parse JSON
      const cleanJson = rawResponse.replace(/```json/g, '').replace(/```/g, '').trim();
      const parsed = JSON.parse(cleanJson);
      const score = Math.max(0, Math.min(100, Number(parsed.score) || 75));
      const feedback = parsed.feedback || 'Evaluasi selesai.';

      return {
        score,
        feedback,
        analysis: inspectedContent,
        prompt,
        isVideo: false,
      };
    } catch (err: any) {
      this.logger.error(`AI Evaluation failed: ${err.message}`);
      return {
        score: 75,
        feedback: `Evaluasi AI otomatis mengalami kendala (${err.message}). Ini adalah nilai draft fallback.`,
        analysis: inspectedContent,
        isVideo: false,
      };
    }
  }
}
