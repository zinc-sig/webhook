import httpClient from "../utils/http";
import redis from "../utils/redis";
import { ADD_REPORT_ARTIFACTS, GET_GRADING_SUBMISSIONS, GET_LATEST_SUBMISSIONS_FOR_ASSIGNMENT_CONFIG, GET_SELECTED_SUBMISSIONS } from "../utils/queries";

export async function generateReportArtifacts(report: any) {
  try {
    const { pipeline_results, is_final } = report;
    const { stageReports, scoreReports } = pipeline_results;
    const stages = Object.keys(stageReports);
    if (stages.length > 0) {
      console.log(`[!] Generating censored grading report and scoring`);
      const censoredReports: any = {};
      let grade;
      for (const stage of stages) {
        switch (stage) {
          case 'valgrind':
            censoredReports[stage] = stageReports[stage].map((report: any) => {
              switch (report.visibility) {
                case 'ALWAYS_HIDDEN':
                  report.stdout = [];
                  report.errors = [];
                  return report;
                case 'VISIBLE_AFTER_GRADING':
                  if (!is_final) {
                    report.stdout = [];
                    report.errors = [];
                    return report;
                  }
                case 'VISIBLE_AFTER_GRADING_IF_FAILED':
                  if (!is_final || !report.isCorrect) {
                    report.stdout = [];
                    report.errors = [];
                    return report;
                  }
                case 'ALWAYS_VISIBLE':
                default:
                  return report;
              }
            })
          case 'stdioTest':
            censoredReports[stage] = stageReports[stage].map((report: any) => {
              switch (report.visibility) {
                case 'ALWAYS_HIDDEN':
                  report.stdout = [];
                  report.expect = [];
                  report.diff = [];
                  return report;
                case 'VISIBLE_AFTER_GRADING':
                  if (!is_final) {
                    report.expect = []
                    report.diff = []
                    return report;
                  }
                case 'VISIBLE_AFTER_GRADING_IF_FAILED':
                  if (!is_final || !report.isCorrect) {
                    report.expect = []
                    report.diff = []
                    return report;
                  }
                case 'ALWAYS_VISIBLE':
                default:
                  return report;
              }
            })
            break;
          case 'score':
            const [score] = stageReports[stage];
            grade = score;
            break;
          default:
            censoredReports[stage] = stageReports[stage];
            break;
        }
      }
      if (scoreReports) {
        grade['details'] = scoreReports;
      }
      const { data } = await httpClient.request({
        url: '/graphql',
        data: {
          query: ADD_REPORT_ARTIFACTS,
          variables: {
            id: report.id,
            sanitizedReports: censoredReports,
            grade
          }
        }
      });
      console.log(`[!] Post-grading artifacts generation completed for report #${report.id}`)
    }
  } catch (error) {
    console.error(`[✗] Error occurred when processing post-grading artifacts for report #${report.id}; Reason: ${error.message}`);
    throw error
  }
}

export async function scheduleGradingEvent(assignment_config_id: number, stop_collection_at: string) {
  try {
    const payload = {
      type: 'create_scheduled_event',
      args: {
        webhook: `http://${process.env.WEBHOOK_ADDR}/trigger/gradingTask`,
        schedule_at: (new Date(stop_collection_at)).toISOString(),
        payload: {
          assignment_config_id,
          stop_collection_at
        }
      }
    };
    await httpClient.request({
      url: '/query',
      data: payload
    });
    console.log(`[!] Scheduled grading task for assignment config ${assignment_config_id} at ${stop_collection_at}`)
  } catch (error) {
    console.error(`[✗] Error occurred when scheduling grading task, reason: ${error.message}`);
    throw error
  }
}

export async function getSelectedSubmissions(submissions: Array<number>, assignmentConfigId: number) {
  try {
    const payload = {
      query: submissions.length === 0 ? GET_LATEST_SUBMISSIONS_FOR_ASSIGNMENT_CONFIG : GET_SELECTED_SUBMISSIONS,
      variables: {
        ...(submissions.length === 0 && { assignmentConfigId }),
        ...(submissions.length !== 0 && { submissions })
      }
    }
    const { data: { data } } = await httpClient.request({
      url: '/graphql',
      data: payload
    });
    return submissions.length === 0 ? data.assignmentConfig.submissions : data.submissions;
  } catch (error) {
    console.error(error);
    throw error;
  }
}

export async function getGradingSubmissions(assignmentConfigId: number) {
  try {
    const { data } = await httpClient.request({
      url: '/graphql',
      data: {
        query: GET_GRADING_SUBMISSIONS,
        variables: {
          assignmentConfigId
        }
      }
    });
    console.log(data);
    const { stopCollectionAt, submissions } = data.data.assignmentConfig;
    return {
      stopCollectionAt,
      submissions
    }
  } catch (error) {
    console.error(error);
    throw error;
  }
}
