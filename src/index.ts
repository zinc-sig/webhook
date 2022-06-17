import { assert } from "console";
import express from "express";
import bodyParser, { json } from "body-parser";
import cookieParser from "cookie-parser";
import { parse } from "cookie";
import nodemailer from "nodemailer";
import smtpTransport from "nodemailer-smtp-transport";
import * as dotenv from 'dotenv';
// import * as admin from 'firebase-admin';
import crypto from "crypto";
import redis from "./utils/redis";

// admin.initializeApp({
//   credential: admin.credential.applicationDefault()
// });

const postalService = nodemailer.createTransport(smtpTransport({
  service: 'gmail',
  host: 'smtp.gmail.com',
  auth: {
    user: process.env.SMTP_USERNAME,
    pass: process.env.SMTP_PASSWORD
  }
}))

if (process.env.NODE_ENV !== 'production') {
  dotenv.config();
  console.log('[!] Loading Env File');
}

import { verifySignature, getUser } from "./lib/user";
import { SyncEnrollment } from "./lib/course";
import { decompressSubmission, getGradingPolicy } from "./lib/decompression"
import { scheduleGradingEvent, getGradingSubmissions, generateReportArtifacts, getSelectedSubmissions } from "./lib/grading";

const port = process.env.WEBHOOK_PORT || 4000;

// import { loadPackageDefinition, credentials } from '@grpc/grpc-js';
// import { loadSync } from '@grpc/proto-loader';
// import { getNotiRecevier, getSubmissionUserId } from "./lib/notification";
// import httpClient from "./utils/http";
// var PROTO_PATH = 'proto/Zinc.proto';
// var target = 'fakegrader:50051';

// interface Noti {
//   student: boolean,
//   id: number,
//   title: string,
//   body: string
// }

// function genGRPCClient() {
//   var packageDefinition = loadSync(
//     PROTO_PATH,
//     {
//       keepCase: true,
//       longs: String,
//       enums: String,
//       defaults: true,
//       oneofs: true
//     });
//   var protoDescriptor = loadPackageDefinition(packageDefinition)
//   var zinc_proto = protoDescriptor.Zinc;
//   //@ts-ignore
//   var client = new zinc_proto(target, credentials.createInsecure());
//   return client
// }

(async () => {
  try {
    const server = express();
    server.use(cookieParser());
    server.use(bodyParser.json({ limit: '50mb' }));


    // var client = genGRPCClient();
    // var call = client.notification({});
    // call.on('data', async function(noti : Noti){
    //   if (!noti.student){
    //     const ids = await getNotiRecevier(noti.id)
    //       console.log(ids)
    //       ids.forEach((id: number) => {
    //         const message = {
    //             data: {
    //               title: noti.title,
    //               body: noti.body,
    //             },
    //             topic: "i"+id+"-"+noti.id.toString()
    //         }
    //         console.log(message)
    //         admin.messaging().send(message).then((response)=>{
    //           console.log('Successfully sent message: ', response)
    //         })
    //         .catch((error)=>{
    //           console.log('Error sending message: ', error)
    //         })
    //       });  
    //   }
    // })

    //   function str_obj(str: string) {
    //     var strArray = str.split('; ');
    //     var result = new Map();
    //     for (let i in strArray) {
    //         const cur = str[i].split('=');
    //         result.set(cur[0], cur[1])
    //     }
    //     return result;
    // }

    server.post(`/identity`, async (req, res) => {
      try {
        if(typeof req.body.headers.Cookie !== "string") {
          console.log(`cookie is not string: ${req.body.headers.Cookie}`)
        }
        const cookies: any = parse(req.body.headers.Cookie)
        if (Object.keys(cookies).length && cookies.hasOwnProperty('appSession')) {
          // const sid = cookieParser.signedCookie(req.cookies['appSession'], process.env.SESSION_SECRET);
          const sid = crypto.createHmac('sha1', process.env.SESSION_SECRET!).update(cookies['appSession']).digest().toString('base64')
          const cookie = await redis.get(sid);
          if (cookie) {
            const { data: { id_token } } = JSON.parse(cookie);
            const { name, itsc } = await verifySignature(id_token, cookies['client'] as string, cookies['domain'] as string);
            const { isAdmin, courses } = await getUser(itsc, name);

            const allowedCourses = `{${courses.map(({ course_id }: any) => course_id).join(',')}}`;
            const payload = {
              'X-Hasura-User-Id': itsc,
              'X-Hasura-Role': isAdmin ? 'admin' : 'user',
              ...(!isAdmin && { 'X-Hasura-Allowed-Courses': allowedCourses }),
              'X-Hasura-Requested-At': (new Date()).toISOString()
            }

            // console.log(req.body)
            res.json(payload);

          } else {
            res.status(401).send('Could not find request session with auth credentials');
          }
        } else {
          res.status(401).send('Unauthorized');
        }
      } catch (error) {
        console.error(`[✗] Error while processing /identity: ${error.message}`)
        res.status(500).json({
          status: 'error',
          error: error.message
        });
      }
    });

    server.post(`/trigger/syncEnrollment`, async (req, res) => {
      try {
        await SyncEnrollment();
        res.json({
          status: 'success'
        });
      } catch (error) {
        console.error(`[✗] Error while processing /trigger/syncEnrollment: ${error.message}`)
        res.status(500).json({
          status: 'error',
          error: error.message
        });
      }
    });

    server.post(`/trigger/decompression`, async (req, res) => {
      try {
        const { event: { data } } = req.body;
        await decompressSubmission(data.new);
        const { gradeImmediately, isTest } = await getGradingPolicy(data.new.assignment_config_id, data.new.user_id);
        if (gradeImmediately) {
          console.log(`[!] Triggered grader for submission id: ${data.new.id}`);
          const payload = JSON.stringify({
            submissions: [
              {
                id: data.new.id,
                extracted_path: `extracted/${data.new.id}`,
                created_at: (new Date(data.new.created_at)).toISOString()
              }
            ],
            assignment_config_id: data.new.assignment_config_id,
            isTest,
            initiatedBy: null
          })
          const clients = await redis.rpush(`zinc_queue:grader`, JSON.stringify({
            job: 'gradingTask',
            payload
          }));
          assert(clients!==0, 'Job signal receiver assertion failed');
          // client.gradingTask(payload, function(err:any ,message: string){
          //   if (err) {
          //     console.log(err)
          //   }
          //   console.log(message)
          // })
        }
        res.json({
          status: 'success'
        })
      } catch (error) {
        console.error(`[✗] Error while processing /trigger/decompression: ${error.message}`)
        res.status(500).json({
          status: 'error',
          error: error.message
        });
      }
    });
    server.post(`/trigger/postGradingProcessing`, async (req, res) => {
      try {
        const { data } = req.body.event;
        console.log(`[!] Received post-grading report processing request for report id: ${data.new.id}`);
        console.log(data.new)
        await generateReportArtifacts(data.new);
        // const id = await getSubmissionUserId(data.new.submission_id)
        // const message = {
        //   data: {
        //     title: "Submission Graded",
        //     body: "submission id : " + data.new.submission_id.toString(),
        //   },
        //   topic: "s" + id + "-" + data.new.submission_id.toString()
        // }
        // admin.messaging().send(message).then((response) => {
        //   console.log('Successfully sent message: ', response)
        // })
        //   .catch((error) => {
        //     console.log('Error sending message: ', error)
        //   })
        res.json({
          status: 'success'
        });
      } catch (error) {
        console.error(`[✗] Error while processing /trigger/postGradingProcessing: ${error.message}`)
        res.status(500).json({
          status: 'error',
          error: error.message
        });
      }
    });

    // server.post(`/trigger/notifications/subscribe/:topic`, async (req, res) => {
    //   try {
    //     const { registrationToken, userId } = req.body;
    //     console.log("subscription request received");
    //     console.log("registration token: " + registrationToken)
    //     console.log("userId: " + userId)
    //     console.log("topic: " + req.params.topic)
    //     console.log(req.body)
    //     const result = await admin.messaging().subscribeToTopic(registrationToken, req.params.topic);
    //     if (result.errors.length != 0) {
    //       console.log(result.errors[0].error)
    //     }
    //     res.json({
    //       status: 'success',
    //       ...result
    //     });
    //   } catch (error) {
    //     console.error(`[✗] Error while subscribing notification for client; reason: ${error.message}`)
    //     res.status(500).json({
    //       status: 'error',
    //       error: error.message
    //     });
    //   }
    // })

    // server.delete(`/trigger/notifications/unsubscribe/:topic`, async (req, res) => {
    //   try {
    //     const { registrationToken } = req.body;
    //     const result = await admin.messaging().unsubscribeFromTopic(registrationToken, req.params.topic);
    //     res.json({
    //       status: 'success',
    //       ...result
    //     });
    //     res.json({
    //       status: 'success'
    //     });
    //   } catch (error) {
    //     console.error(`[✗] Error while unsubscribing notification for client; reason: ${error.message}`)
    //     res.status(500).json({
    //       status: 'error',
    //       error: error.message
    //     });
    //   }
    // });

    server.post(`/trigger/scheduleGrading`, async (req, res) => {
      try {
        const { op, data } = req.body.event;
        if ((op === 'UPDATE' && data.old.stop_collection_at !== data.new.stop_collection_at) || op === 'INSERT') {
          await scheduleGradingEvent(data.new.id, data.new.stop_collection_at);
        }
        res.json({
          status: 'success'
        })
      } catch (error) {
        console.error(`[✗] Error while processing /trigger/scheduleGrading: ${error.message}`)
        res.status(500).json({
          status: 'error',
          error: error.message
        });
      }
    });

    server.post(`/trigger/manualGradingTask/:assignmentConfigId`, async (req, res) => {
      try {
        const { assignmentConfigId } = req.params;
        console.log(`[!] Received manual grading task for assignment config #${assignmentConfigId} for submissions [${req.body.submissions.toString()}]`)
        const submissions = await getSelectedSubmissions(req.body.submissions, parseInt(assignmentConfigId, 10));
        console.log(`[!] Retreived ${submissions.length} submissions for assignment config #${assignmentConfigId}'s grading request `);
        const payload = JSON.stringify({
          submissions: submissions.map((submission: any) => ({ ...submission, created_at: (new Date(submission.created_at)).toISOString() })),
          assignment_config_id: parseInt(assignmentConfigId, 10),
          isTest: false,
          initiatedBy: req.body.initiatedBy
        });
        // client.gradingTask(payload, function(err:any ,message: string){
        //   if (err) {
        //     console.log(err)
        //   }
        //   console.log(message)
        // })
        const clients = await redis.rpush(`zinc_queue:grader`, JSON.stringify({
          job: 'gradingTask',
          payload 
        }));
        assert(clients!==0, 'Job signal receiver assertion failed');
        res.json({
          status: 'success'
        });
      } catch (error) {
        console.error(`[✗] Error while processing /trigger/manualGradingTask: ${error.message}`);
        res.status(500).json({
          status: 'error',
          error: error.message
        });
      }
    });

    server.post(`/trigger/gradingTask`, async (req, res) => {
      try {
        const { assignment_config_id, stop_collection_at } = req.body.payload
        const { submissions, stopCollectionAt } = await getGradingSubmissions(assignment_config_id);
        if (stopCollectionAt === stop_collection_at) {
          console.log('inside grading Task')
          const payload = {
            submissions: submissions.map((submission: any) => ({ ...submission, created_at: (new Date(submission.created_at)).toISOString() })),
            assignment_config_id,
            isTest: false,
            // initiatedBy: null
          };
          const clients = await redis.rpush(`zinc_queue:grader`, JSON.stringify({
            job: 'gradingTask',
            payload 
          }));
          assert(clients!==0, 'Job signal receiver assertion failed');
          // client.gradingTask(payload, function(err:any ,message: string){
          //   if (err) {
          //     console.log(err)
          //   }
          //   console.log(message)
          // })
        }
        res.json({
          status: 'success'
        });
      } catch (error) {
        console.error(`[✗] Error while processing /trigger/gradingTask: ${error.message}`);
        res.status(500).json({
          status: 'error',
          error: error.message
        });
      }
    });

    async function doneGradingSignalPolling() {
      console.log(`[!] Started Polling for grading job signal`);
      while (true) {
        try {
          const task = await redis.blpop('doneGrading', 0);
          console.log(`[Done Grading] ${JSON.stringify(task)}`);
          // const mailOptions = {
          //   from: process.env.MAIL_AUTHOR,
          //   to: '',
          //   subject: '',
          //   html: ''
          // }
          // postalService.sendMail(mailOptions, (err, info) => {
          //   if(!err) {
          //     console.log(`[!] Mail delivery in progress, ${info}`);
          //   } else {
          //     console.error(`[✗] Error while sending out mail for grading task completion notification`);
          //   }
          // });
        } catch (error) {
          // Redis connect could have closed. Handle those cases here.      
          process.exit(1);
        }
      }
    }

    server.listen(port, (err?: any) => {
      if (err) throw err;
      console.log(`> Ready on localhost:${port} - env ${process.env.NODE_ENV}`);
    });
  } catch (e) {
    console.error(e);
    process.exit(1);
  }
})();
