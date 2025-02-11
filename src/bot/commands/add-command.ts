import TelegramBot from 'node-telegram-bot-api'
import { SUB_MENU } from '../../config/bot-menus'
import { PublicKey } from '@solana/web3.js'
import { PrismaWalletRepository } from '../../repositories/prisma/wallet'
import { userExpectingWalletAddress } from '../../constants/flags'
import { BotMiddleware } from '../../config/bot-middleware'
import { InlineKeyboardMarkup } from 'node-telegram-bot-api'

// Add filter keyboard
const FILTER_MENU: InlineKeyboardMarkup = {
  inline_keyboard: [
    [
      { text: '🔄 All Transactions', callback_data: 'filter_all' },
      { text: '💰 Only Buys', callback_data: 'filter_buys' }
    ],
    [
      { text: '💸 Only Sells', callback_data: 'filter_sells' },
      { text: '🔥 High Value', callback_data: 'filter_high_value' }
    ],
    [{ text: 'Back', callback_data: 'back_to_main_menu' }]
  ]
}

export class AddCommand {
  private prismaWalletRepository: PrismaWalletRepository
  
  constructor(private bot: TelegramBot) {
    this.bot = bot
    this.prismaWalletRepository = new PrismaWalletRepository()
  }

  public addCommandHandler() {
    this.bot.onText(/\/add/, async (msg) => {
      const chatId = msg.chat.id
      const userId = String(msg.from?.id)
      this.add({ message: msg, isButton: false })
    })
  }

  public addButtonHandler(msg: TelegramBot.Message) {
    this.add({ message: msg, isButton: true })
  }

  private add({ message, isButton }: { message: TelegramBot.Message; isButton: boolean }) {
    try {
      const userId = message.chat.id.toString()

      // Simple add message
      const addMessage = '🐱 Please send me the Solana wallet address you want to track'
      
      if (isButton) {
        this.bot.editMessageText(addMessage, {
          chat_id: message.chat.id,
          message_id: message.message_id,
          reply_markup: SUB_MENU,
          parse_mode: 'HTML',
        })
      } else {
        this.bot.sendMessage(message.chat.id, addMessage, {
          reply_markup: SUB_MENU,
          parse_mode: 'HTML',
        })
      }

      userExpectingWalletAddress[Number(userId)] = true
      
      const listener = async (responseMsg: TelegramBot.Message) => {
        if (!userExpectingWalletAddress[Number(userId)]) return

        const text = responseMsg.text
        if (!text || text.startsWith('/')) return

        const walletAddress = text.trim()

        // Basic validation
        try {
          const publicKeyWallet = new PublicKey(walletAddress)
          if (!PublicKey.isOnCurve(publicKeyWallet.toBytes())) {
            this.bot.sendMessage(message.chat.id, `😾 Not a valid Solana wallet address`)
            return
          }
        } catch {
          this.bot.sendMessage(message.chat.id, `😾 Not a valid Solana wallet address`)
          return
        }

        // Check if already tracking
        const isWalletAlready = await this.prismaWalletRepository.getUserWalletById(userId, walletAddress)
        if (isWalletAlready) {
          this.bot.sendMessage(message.chat.id, `🐱 You're already tracking this wallet`)
          return
        }

        // Add wallet
        await this.prismaWalletRepository.create(userId, walletAddress)
        this.bot.sendMessage(message.chat.id, 
          `✅ Now tracking wallet: ${walletAddress}\n\nChoose which transactions you want to track:`, {
          reply_markup: FILTER_MENU,
          parse_mode: 'HTML'
        })

        // Cleanup
        this.bot.removeListener('message', listener)
        userExpectingWalletAddress[Number(userId)] = false
      }

      this.bot.once('message', listener)
    } catch (error) {
      this.bot.sendMessage(message.chat.id, `😿 Something went wrong! Please try again`)
    }
  }
}
