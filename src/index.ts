import { assert } from "console";
import express from "express";
import bodyParser, { json } from "body-parser";
import cookieParser from "cookie-parser";
import { parse } from "cookie";
import * as dotenv from "dotenv";
import crypto from "crypto";
import redis from "./utils/redis";

if (process.env.NODE_ENV !== "production") {
  dotenv.config();
  console.log("[!] Loading Env File");
}

import { verifySignature, getUser } from "./lib/user";
import { SyncEnrollment } from "./lib/course";
import { decompressSubmission, getGradingPolicy } from "./lib/decompression";
import {
  scheduleGradingEvent,
  getGradingSubmissions,
  generateReportArtifacts,
  getSelectedSubmissions,
} from "./lib/grading";

const port = process.env.WEBHOOK_PORT || 4000;

(async () => {
  try {
    const server = express();
    server.use(cookieParser());
    server.use(bodyParser.json({ limit: "50mb" }));

    server.post(`/identity`, async (req, res) => {
      try {
        const cookies: any = parse(req.body.headers.Cookie);
        if (
          Object.keys(cookies).length &&
          cookies.hasOwnProperty("appSession")
        ) {
          const sid = crypto
            .createHmac("sha1", process.env.SESSION_SECRET!)
            .update(cookies["appSession"])
            .digest()
            .toString("base64");
          const cookie = await redis.get(sid);
          if (cookie) {
            const {
              data: { id_token },
            } = JSON.parse(cookie);
            const { name, itsc } = await verifySignature(
              id_token,
              cookies["client"] as string,
              cookies["domain"] as string
            );
            const { isAdmin, courses } = await getUser(itsc, name);

            const allowedCourses = `{${courses
              .map(({ course_id }: any) => course_id)
              .join(",")}}`;
            const payload = {
              "X-Hasura-User-Id": itsc,
              "X-Hasura-Role": isAdmin ? "admin" : "user",
              ...(!isAdmin && { "X-Hasura-Allowed-Courses": allowedCourses }),
              "X-Hasura-Requested-At": new Date().toISOString(),
            };

            res.json(payload);
          } else {
            res
              .status(401)
              .send("Could not find request session with auth credentials");
          }
        } else {
          res.status(401).send("Unauthorized");
        }
      } catch (error) {
        console.error(`[✗] Error while processing /identity: ${error.message}`);
        res.status(500).json({
          status: "error",
          error: error.message,
        });
      }
    });

    server.post(`/trigger/syncEnrollment`, async (req, res) => {
      try {
        await SyncEnrollment();
        res.json({
          status: "success",
        });
      } catch (error) {
        console.error(
          `[✗] Error while processing /trigger/syncEnrollment: ${error.message}`
        );
        res.status(500).json({
          status: "error",
          error: error.message,
        });
      }
    });

    server.post(`/trigger/decompression`, async (req, res) => {
      try {
        const {
          event: { data },
        } = req.body;
        await decompressSubmission(data.new);
        const { gradeImmediately, isTest } = await getGradingPolicy(
          data.new.assignment_config_id,
          data.new.user_id
        );
        if (gradeImmediately) {
          console.log(`[!] Triggered grader for submission id: ${data.new.id}`);
          const payload = JSON.stringify({
            submissions: [
              {
                id: data.new.id,
                extracted_path: `extracted/${data.new.id}`,
                created_at: new Date(data.new.created_at).toISOString(),
              },
            ],
            assignment_config_id: data.new.assignment_config_id,
            isTest,
            initiatedBy: null,
          });
          const clients = await redis.rpush(
            `zinc_queue:grader`,
            JSON.stringify({
              job: "gradingTask",
              payload,
            })
          );
          assert(clients !== 0, "Job signal receiver assertion failed");
        }
        res.json({
          status: "success",
        });
      } catch (error) {
        console.error(
          `[✗] Error while processing /trigger/decompression: ${error.message}`
        );
        res.status(500).json({
          status: "error",
          error: error.message,
        });
      }
    });
    server.post(`/trigger/postGradingProcessing`, async (req, res) => {
      try {
        const { data } = req.body.event;
        console.log(
          `[!] Received post-grading report processing request for report id: ${data.new.id}`
        );
        await generateReportArtifacts(data.new);
        res.json({
          status: "success",
        });
      } catch (error) {
        console.error(
          `[✗] Error while processing /trigger/postGradingProcessing: ${error.message}`
        );
        res.status(500).json({
          status: "error",
          error: error.message,
        });
      }
    });

    server.post(`/trigger/scheduleGrading`, async (req, res) => {
      try {
        const { op, data } = req.body.event;
        if (
          (op === "UPDATE" &&
            data.old.stop_collection_at !== data.new.stop_collection_at) ||
          op === "INSERT"
        ) {
          await scheduleGradingEvent(data.new.id, data.new.stop_collection_at);
        }
        res.json({
          status: "success",
        });
      } catch (error) {
        console.error(
          `[✗] Error while processing /trigger/scheduleGrading: ${error.message}`
        );
        res.status(500).json({
          status: "error",
          error: error.message,
        });
      }
    });

    server.post(
      `/trigger/manualGradingTask/:assignmentConfigId`,
      async (req, res) => {
        try {
          const { assignmentConfigId } = req.params;
          console.log(
            `[!] Received manual grading task for assignment config #${assignmentConfigId} for submissions [${req.body.submissions.toString()}]`
          );
          const submissions = await getSelectedSubmissions(
            req.body.submissions,
            parseInt(assignmentConfigId, 10)
          );
          console.log(
            `[!] Retreived ${submissions.length} submissions for assignment config #${assignmentConfigId}'s grading request `
          );
          const payload = JSON.stringify({
            submissions: submissions.map((submission: any) => ({
              ...submission,
              created_at: new Date(submission.created_at).toISOString(),
            })),
            assignment_config_id: parseInt(assignmentConfigId, 10),
            isTest: false,
            initiatedBy: req.body.initiatedBy,
          });
          const clients = await redis.rpush(
            `zinc_queue:grader`,
            JSON.stringify({
              job: "gradingTask",
              payload,
            })
          );
          assert(clients !== 0, "Job signal receiver assertion failed");
          res.json({
            status: "success",
          });
        } catch (error) {
          console.error(
            `[✗] Error while processing /trigger/manualGradingTask: ${error.message}`
          );
          res.status(500).json({
            status: "error",
            error: error.message,
          });
        }
      }
    );

    server.post(`/trigger/gradingTask`, async (req, res) => {
      try {
        const { assignment_config_id, stop_collection_at } = req.body.payload;
        const { submissions, stopCollectionAt } = await getGradingSubmissions(
          assignment_config_id
        );
        if (stopCollectionAt === stop_collection_at) {
          const payload = {
            submissions: submissions.map((submission: any) => ({
              ...submission,
              created_at: new Date(submission.created_at).toISOString(),
            })),
            assignment_config_id,
            isTest: false,
            // initiatedBy: null
          };
          const clients = await redis.rpush(
            `zinc_queue:grader`,
            JSON.stringify({
              job: "gradingTask",
              payload,
            })
          );
          assert(clients !== 0, "Job signal receiver assertion failed");
        }
        res.json({
          status: "success",
        });
      } catch (error) {
        console.error(
          `[✗] Error while processing /trigger/gradingTask: ${error.message}`
        );
        res.status(500).json({
          status: "error",
          error: error.message,
        });
      }
    });

    server.listen(port, (err?: any) => {
      if (err) throw err;
      // setInterval(async () => {
      //   const [key, result] = await redis.blpop("zinc-queue:api", 4000);
      //   const payload = JSON.parse(result);
      //   console.log(payload);
      // }, 3000);
      console.log(`> Ready on localhost:${port} - env ${process.env.NODE_ENV}`);
    });
  } catch (e) {
    console.error(e);
    process.exit(1);
  }
})();
